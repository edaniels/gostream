package main

import (
	"context"
	"flag"
	"image"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/edaniels/golog"
	"github.com/edaniels/gostream"
	"github.com/edaniels/gostream/codec/vpx"
	fyneutils "github.com/edaniels/gostream/utils/fyne"

	"fyne.io/fyne"
	"fyne.io/fyne/canvas"
	"fyne.io/fyne/test"
	"fyne.io/fyne/theme"
)

type viewApp struct {
	fyne.App
	mainWindow fyne.Window
}

func newViewApp(name string, img image.Image) (*viewApp, error) {
	app := test.NewApp()
	app.Settings().SetTheme(theme.DarkTheme())
	window := app.NewWindow(name)
	window.SetPadded(false)

	canvasImg := canvas.NewImageFromImage(img)
	bounds := img.Bounds()
	canvasImg.SetMinSize(fyne.Size{bounds.Max.X, bounds.Max.Y})
	window.SetContent(canvasImg)

	return &viewApp{
		App:        app,
		mainWindow: window,
	}, nil
}

func view(port int, name string, img image.Image) error {
	remoteView, err := gostream.NewRemoteView(vpx.DefaultRemoteViewConfig)
	if err != nil {
		return err
	}

	app, err := newViewApp(name, img)
	if err != nil {
		return err
	}
	remoteView.SetOnClickHandler(func(x, y int) {
		golog.Global.Debugw("click", "x", x, "y", y)
	})

	server := gostream.NewRemoteViewServer(port, remoteView, golog.Global)
	server.Run()

	cancelCtx, cancelFunc := context.WithCancel(context.Background())
	c := make(chan os.Signal, 2)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	go fyneutils.StreamWindow(cancelCtx, app.mainWindow, remoteView, 250*time.Millisecond)
	app.mainWindow.ShowAndRun()

	<-c
	cancelFunc()
	app.mainWindow.Close()
	remoteView.Stop()
	return nil
}

func main() {
	imagePath := flag.String("image", "", "image to show")
	port := flag.Int("port", 5555, "port to run server on")
	flag.Parse()

	if *imagePath == "" {
		flag.Usage()
		os.Exit(1)
	}

	imageFile, err := os.Open(*imagePath)
	if err != nil {
		golog.Global.Fatal(err)
	}

	img, _, err := image.Decode(imageFile)
	if err != nil {
		golog.Global.Fatal(err)
	}

	if err := view(*port, imageFile.Name(), img); err != nil {
		golog.Global.Error(err)
	}
}
