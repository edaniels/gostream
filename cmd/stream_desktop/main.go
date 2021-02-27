package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/edaniels/golog"
	"github.com/edaniels/gostream"
	"github.com/edaniels/gostream/codec/vpx"
	"github.com/edaniels/gostream/codec/x264"
)

func main() {
	port := flag.Int("port", 5555, "port to run server on")
	camera := flag.Bool("camera", false, "use camera")
	dupeView := flag.Bool("dupe_view", false, "duplicate view")
	dupeStream := flag.Bool("dupe_stream", false, "duplicate stream")
	extraTiles := flag.Int("extra_tiles", 0, "number of times to duplicate screen in tiles")
	flag.Parse()

	var videoReader gostream.VideoReadCloser
	var err error
	if *camera {
		videoReader, err = gostream.GetUserReader()

	} else {
		videoReader, err = gostream.GetDisplayReader()
	}
	if err != nil {
		panic(err)
	}

	defer func() {
		if err := videoReader.Close(); err != nil {
			golog.Global.Error(err)
		}
	}()

	_ = x264.DefaultRemoteViewConfig
	_ = vpx.DefaultRemoteViewConfig
	config := vpx.DefaultRemoteViewConfig
	config.Debug = false
	remoteView, err := gostream.NewRemoteView(config)
	if err != nil {
		panic(err)
	}

	remoteView.SetOnDataHandler(func(data []byte) {
		golog.Global.Debugw("data", "raw", string(data))
		remoteView.SendText(string(data))
	})
	remoteView.SetOnClickHandler(func(x, y int) {
		golog.Global.Debugw("click", "x", x, "y", y)
		remoteView.SendText(fmt.Sprintf("got click (%d, %d)", x, y))
	})

	server := gostream.NewRemoteViewServer(*port, remoteView, golog.Global)
	var secondView gostream.RemoteView
	if *dupeView {
		config.StreamName = "dupe"
		config.StreamNumber = 1
		remoteView, err := gostream.NewRemoteView(config)
		if err != nil {
			panic(err)
		}

		remoteView.SetOnDataHandler(func(data []byte) {
			golog.Global.Debugw("data", "raw", string(data))
			remoteView.SendText(string(data))
		})
		remoteView.SetOnClickHandler(func(x, y int) {
			golog.Global.Debugw("click", "x", x, "y", y)
			remoteView.SendText(fmt.Sprintf("got click (%d, %d)", x, y))
		})
		secondView = remoteView
		server.AddView(secondView)
	}
	server.Run()

	cancelCtx, cancelFunc := context.WithCancel(context.Background())
	c := make(chan os.Signal, 2)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		cancelFunc()
	}()

	captureRate := 33 * time.Millisecond
	imgSrc := gostream.VideoReadReleaser{videoReader}
	if secondView != nil {
		go gostream.StreamSource(cancelCtx, imgSrc, secondView, captureRate)
	}
	if *dupeStream {
		go gostream.StreamNamedSource(cancelCtx, imgSrc, "dupe", remoteView, captureRate)
	}
	if *extraTiles == 0 {
		gostream.StreamNamedSource(cancelCtx, imgSrc, "screen", remoteView, captureRate)
	} else {
		autoTiler := gostream.NewAutoTiler(800, 600, imgSrc)
		for i := 0; i < *extraTiles; i++ {
			autoTiler.AddSource(imgSrc)
		}
		gostream.StreamNamedSource(cancelCtx, autoTiler, "tiled screens", remoteView, captureRate)
	}
	if err := server.Stop(context.Background()); err != nil {
		golog.Global.Error(err)
	}
}
