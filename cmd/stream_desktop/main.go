package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/edaniels/gostream"
	"github.com/edaniels/gostream/codec/vpx"
	"github.com/edaniels/gostream/codec/x264"
	"github.com/edaniels/gostream/media"
)

func main() {
	port := flag.Int("port", 5555, "port to run server on")
	camera := flag.Bool("camera", false, "use camera")
	dupeView := flag.Bool("dupe_view", false, "duplicate view")
	dupeStream := flag.Bool("dupe_stream", false, "duplicate stream")
	extraTiles := flag.Int("extra_tiles", 0, "number of times to duplicate screen in tiles")
	flag.Parse()

	var videoReader media.VideoReadCloser
	var err error
	if *camera {
		videoReader, err = media.GetAnyVideoReader(media.DefaultConstraints)
	} else {
		videoReader, err = media.GetAnyScreenReader(media.DefaultConstraints)
	}
	if err != nil {
		gostream.Logger.Fatal(err)
	}

	defer func() {
		if err := videoReader.Close(); err != nil {
			gostream.Logger.Error(err)
		}
	}()

	_ = x264.DefaultViewConfig
	_ = vpx.DefaultViewConfig
	config := vpx.DefaultViewConfig
	view, err := gostream.NewView(config)
	if err != nil {
		gostream.Logger.Fatal(err)
	}

	view.SetOnDataHandler(func(data []byte, responder gostream.ClientResponder) {
		gostream.Logger.Debugw("data", "raw", string(data))
		responder.SendText(string(data))
	})
	view.SetOnClickHandler(func(x, y int, responder gostream.ClientResponder) {
		gostream.Logger.Debugw("click", "x", x, "y", y)
		responder.SendText(fmt.Sprintf("got click (%d, %d)", x, y))
	})

	server := gostream.NewViewServer(*port, view, gostream.Logger)
	var secondView gostream.View
	if *dupeView {
		config.StreamName = "dupe"
		config.StreamNumber = 1
		view, err := gostream.NewView(config)
		if err != nil {
			gostream.Logger.Fatal(err)
		}

		view.SetOnDataHandler(func(data []byte, responder gostream.ClientResponder) {
			gostream.Logger.Debugw("data", "raw", string(data))
			responder.SendText(string(data))
		})
		view.SetOnClickHandler(func(x, y int, responder gostream.ClientResponder) {
			gostream.Logger.Debugw("click", "x", x, "y", y)
			responder.SendText(fmt.Sprintf("got click (%d, %d)", x, y))
		})
		secondView = view
		server.AddView(secondView)
	}
	if err := server.Start(); err != nil {
		gostream.Logger.Fatal(err)
	}

	cancelCtx, cancelFunc := context.WithCancel(context.Background())
	c := make(chan os.Signal, 2)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		cancelFunc()
	}()

	if secondView != nil {
		go gostream.StreamSource(cancelCtx, videoReader, secondView)
	}
	if *dupeStream {
		go gostream.StreamNamedSource(cancelCtx, videoReader, "dupe", view)
	}
	if *extraTiles == 0 {
		gostream.StreamNamedSource(cancelCtx, videoReader, "screen", view)
	} else {
		autoTiler := gostream.NewAutoTiler(800, 600, videoReader)
		for i := 0; i < *extraTiles; i++ {
			autoTiler.AddSource(videoReader)
		}
		gostream.StreamNamedSource(cancelCtx, autoTiler, "tiled screens", view)
	}
	if err := server.Stop(context.Background()); err != nil {
		gostream.Logger.Error(err)
	}
}
