package gostream

import (
	"context"
)

// streamSource will stream a source of images forver to the view until the given context tells it to cancel.
func streamSource(ctx context.Context, once func(), is ImageSource, name string, view View) {
	if once != nil {
		once()
	}
	stream := view.ReserveStream(name)
	select {
	case <-ctx.Done():
		return
	case <-view.StreamingReady():
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

// StreamSource streams the given image source to the view forever until context signals cancelation.
func StreamSource(ctx context.Context, is ImageSource, view View) {
	streamSource(ctx, nil, is, "", view)
}

// StreamNamedSource streams the given image source to the view forever until context signals cancelation.
// The given name is used to identify the stream.
func StreamNamedSource(ctx context.Context, is ImageSource, name string, view View) {
	streamSource(ctx, nil, is, name, view)
}
