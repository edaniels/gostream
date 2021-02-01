package fyneutils

import (
	"context"
	"image"
	"time"

	"github.com/edaniels/gostream"

	"fyne.io/fyne"
)

func ImageSourceOfWindow(window fyne.Window) gostream.ImageSource {
	return gostream.ImageSourceFunc(func(ctx context.Context) (image.Image, error) {
		return window.Canvas().Capture(), nil
	})
}

func StreamWindow(ctx context.Context, window fyne.Window, remoteView gostream.RemoteView, captureInternal time.Duration) {
	gostream.StreamSourceOnce(
		ctx,
		func() { time.Sleep(2 * time.Second) },
		ImageSourceOfWindow(window),
		remoteView,
		captureInternal,
	)
}
