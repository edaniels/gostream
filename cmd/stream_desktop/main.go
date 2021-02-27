package main

import (
	"context"
	"flag"
	"fmt"
	"image"
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
	capture := func(ctx context.Context) (image.Image, error) {
		img, release, err := videoReader.Read()
		if err != nil {
			return nil, err
		}
		cloned := gostream.CloneImage(img)
		release()
		return cloned, nil
	}
	if secondView != nil {
		go gostream.StreamFunc(cancelCtx, capture, secondView, captureRate)
	}
	if *dupeStream {
		go gostream.StreamNamedFunc(cancelCtx, capture, "dupe", remoteView, captureRate)
	}
	if *extraTiles == 0 {
		gostream.StreamNamedFunc(cancelCtx, capture, "screen", remoteView, captureRate)
	} else {
		autoTiler := gostream.NewAutoTiler(800, 600, gostream.ImageSourceFunc(capture))
		for i := 0; i < *extraTiles; i++ {
			autoTiler.AddSource(gostream.ImageSourceFunc(capture))
		}
		gostream.StreamNamedSource(cancelCtx, autoTiler, "tiled screens", remoteView, captureRate)
	}
	if err := server.Stop(context.Background()); err != nil {
		golog.Global.Error(err)
	}
}
