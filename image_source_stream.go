package gostream

import (
	"context"
)

// streamSource will stream a source of images forever to the stream until the given context tells it to cancel.
func streamSource(ctx context.Context, once func(), is ImageSource, stream Stream) {
	if once != nil {
		once()
	}
	select {
	case <-ctx.Done():
		return
	case <-stream.StreamingReady():
	}
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		frame, release, err := is.Next(ctx)
		if err != nil {
			Logger.Debugw("error getting frame", "error", err)
			continue
		}
		select {
		case <-ctx.Done():
			return
		case stream.InputFrames() <- FrameReleasePair{frame, release}:
		}
	}
}

// StreamSource streams the given image source to the stream forever until context signals cancellation.
func StreamSource(ctx context.Context, is ImageSource, stream Stream) {
	streamSource(ctx, nil, is, stream)
}
