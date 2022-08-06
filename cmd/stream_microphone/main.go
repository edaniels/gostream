package main

import (
	"context"

	"github.com/edaniels/golog"

	// register screen drivers.
	_ "github.com/pion/mediadevices/pkg/driver/screen"
	"go.uber.org/multierr"
	"go.viam.com/utils"

	"github.com/edaniels/gostream"
	"github.com/edaniels/gostream/codec/vpx"
	"github.com/edaniels/gostream/codec/x264"
	"github.com/edaniels/gostream/media"
)

func main() {
	utils.ContextualMain(mainWithArgs, logger)
}

var (
	defaultPort = 5555
	logger      = golog.Global.Named("server")
)

// Arguments for the command.
type Arguments struct {
	Port       utils.NetPortFlag `flag:"0"`
	Camera     bool              `flag:"camera,usage=use camera"`
	DupeStream bool              `flag:"dupe_stream,usage=duplicate stream"`
}

func mainWithArgs(ctx context.Context, args []string, logger golog.Logger) error {
	var argsParsed Arguments
	if err := utils.ParseFlags(args, &argsParsed); err != nil {
		return err
	}
	if argsParsed.Port == 0 {
		argsParsed.Port = utils.NetPortFlag(defaultPort)
	}

	return runServer(
		ctx,
		int(argsParsed.Port),
		argsParsed.Camera,
		argsParsed.DupeStream,
		logger,
	)
}

func runServer(
	ctx context.Context,
	port int,
	camera bool,
	dupeStream bool,
	logger golog.Logger,
) (err error) {
	audioReader, err := media.GetAnyAudioReader(media.DefaultConstraints)
	if err != nil {
		return err
	}
	defer func() {
		err = multierr.Combine(err, audioReader.Close())
	}()

	_ = x264.DefaultStreamConfig
	_ = vpx.DefaultStreamConfig
	config := vpx.DefaultStreamConfig
	stream, err := gostream.NewStream(config)
	if err != nil {
		return err
	}
	server, err := gostream.NewStandaloneStreamServer(port, logger, stream)
	if err != nil {
		return err
	}
	if err := server.Start(ctx); err != nil {
		return err
	}

	gostream.StreamSource(ctx, audioReader, stream)
	if err := server.Stop(ctx); err != nil {
		return err
	}
	return nil
}
