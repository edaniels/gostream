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

	"github.com/kbinani/screenshot"
)

func main() {
	port := flag.Int("port", 5555, "port to run server on")
	flag.Parse()

	flag.Parse()

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
	server.Run()

	cancelCtx, cancelFunc := context.WithCancel(context.Background())
	c := make(chan os.Signal, 2)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		cancelFunc()
	}()

	bounds := screenshot.GetDisplayBounds(0)
	gostream.StreamFunc(cancelCtx, func() image.Image {
		img, err := screenshot.CaptureRect(bounds)
		if err != nil {
			panic(err)
		}
		return img
	}, remoteView, 33*time.Millisecond)
	if err := server.Stop(context.Background()); err != nil {
		golog.Global.Error(err)
	}
}
