package fyneutils

import (
	"context"
	"image"
	"time"

	"github.com/edaniels/gostream"

	"fyne.io/fyne"
)

func StreamWindow(ctx context.Context, window fyne.Window, remoteView gostream.RemoteView, captureInternal time.Duration) {
	gostream.StreamFuncOnce(
		ctx,
		func() { time.Sleep(2 * time.Second) },
		func() image.Image { return window.Canvas().Capture() },
		remoteView,
		captureInternal,
	)
}
