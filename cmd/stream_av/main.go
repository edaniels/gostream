package main

import (
	"context"
	"image"

	"github.com/edaniels/golog"
	// register drivers.
	_ "github.com/pion/mediadevices/pkg/driver/camera"
	_ "github.com/pion/mediadevices/pkg/driver/microphone"
	_ "github.com/pion/mediadevices/pkg/driver/screen"
	"go.uber.org/multierr"
	"go.viam.com/utils"

	"github.com/edaniels/gostream"
	"github.com/edaniels/gostream/codec/opus"
	"github.com/edaniels/gostream/codec/vpx"
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
	Port   utils.NetPortFlag `flag:"0"`
	Camera bool              `flag:"camera,usage=use camera"`
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
		logger,
	)
}

func runServer(
	ctx context.Context,
	port int,
	camera bool,
	logger golog.Logger,
) (err error) {
	audioReader, err := media.GetAnyAudioReader(media.DefaultConstraints)
	if err != nil {
		return err
	}
	defer func() {
		err = multierr.Combine(err, audioReader.Close())
	}()
	var videoReader media.ReadCloser[image.Image]
	if camera {
		videoReader, err = media.GetAnyVideoReader(media.DefaultConstraints)
	} else {
		videoReader, err = media.GetAnyScreenReader(media.DefaultConstraints)
	}
	if err != nil {
		return err
	}
	defer func() {
		err = multierr.Combine(err, videoReader.Close())
	}()

	var config gostream.StreamConfig
	config.AudioEncoderFactory = opus.NewEncoderFactory()
	config.VideoEncoderFactory = vpx.NewEncoderFactory(vpx.Version8)
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

	audioErr := make(chan error)
	defer func() {
		err = multierr.Combine(err, <-audioErr, server.Stop(ctx))
	}()

	go func() {
		audioErr <- gostream.StreamAudioSource(ctx, audioReader, stream)
	}()
	return gostream.StreamImageSource(ctx, videoReader, stream)
}
