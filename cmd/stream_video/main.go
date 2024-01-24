// Package main streams video.
package main

import (
	"context"

	"github.com/edaniels/golog"
	utils "github.com/edaniels/goutils"

	// register video drivers.
	_ "github.com/pion/mediadevices/pkg/driver/camera"
	_ "github.com/pion/mediadevices/pkg/driver/screen"
	"go.uber.org/multierr"
	vutils "go.viam.com/utils"

	"github.com/edaniels/gostream"
	"github.com/edaniels/gostream/codec/vpx"
	"github.com/edaniels/gostream/codec/x264"
)

func main() {
	utils.ContextualMain(mainWithArgs, logger)
}

var (
	defaultPort = 5555
	logger      = golog.Global().Named("server")
)

// Arguments for the command.
type Arguments struct {
	Port       vutils.NetPortFlag `flag:"0"`
	Camera     bool               `flag:"camera,usage=use camera"`
	DupeStream bool               `flag:"dupe_stream,usage=duplicate stream"`
	Dump       bool               `flag:"dump"`
}

func mainWithArgs(ctx context.Context, args []string, logger golog.Logger) error {
	var argsParsed Arguments
	if err := vutils.ParseFlags(args, &argsParsed); err != nil {
		return err
	}
	if argsParsed.Dump {
		var all []gostream.DeviceInfo
		if argsParsed.Camera {
			all = gostream.QueryVideoDevices()
		} else {
			all = gostream.QueryScreenDevices()
		}
		for _, info := range all {
			logger.Debugf("%s", info.ID)
			logger.Debugf("\t labels: %v", info.Labels)
			logger.Debugf("\t priority: %v", info.Priority)
			for _, p := range info.Properties {
				logger.Debugf("\t %+v", p.Video)
			}
		}
		return nil
	}
	if argsParsed.Port == 0 {
		argsParsed.Port = vutils.NetPortFlag(defaultPort)
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
	var videoSource gostream.VideoSource
	if camera {
		videoSource, err = gostream.GetAnyVideoSource(gostream.DefaultConstraints, logger)
	} else {
		videoSource, err = gostream.GetAnyScreenSource(gostream.DefaultConstraints, logger)
	}
	if err != nil {
		return utils.ErrorWithStack(err)
	}
	defer func() {
		err = multierr.Combine(err, videoSource.Close(ctx))
	}()

	_ = x264.DefaultStreamConfig
	_ = vpx.DefaultStreamConfig
	config := vpx.DefaultStreamConfig
	stream, err := gostream.NewStream(config)
	if err != nil {
		return err
	}
	server, err := gostream.NewStandaloneStreamServer(port, logger, nil, stream)
	if err != nil {
		return err
	}
	var secondStream gostream.Stream
	if dupeStream {
		config.Name = "dupe"
		stream, err := gostream.NewStream(config)
		if err != nil {
			logger.Fatal(err)
		}

		secondStream = stream
		if err := server.AddStream(secondStream); err != nil {
			return err
		}
	}
	if err := server.Start(ctx); err != nil {
		return err
	}

	secondErr := make(chan error)
	defer func() {
		err = multierr.Combine(err, <-secondErr, server.Stop(ctx))
	}()

	if secondStream != nil {
		go func() {
			secondErr <- gostream.StreamVideoSource(ctx, videoSource, secondStream)
		}()
	} else {
		close(secondErr)
	}
	return gostream.StreamVideoSource(ctx, videoSource, stream)
}
