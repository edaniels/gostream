package gostream

import (
	"context"
)

type ErrorHandler interface {
	// HandleError receives the error returned by ImageSource.Next
	// regardless of whether or not the error is nil (This allows
	// for error handling logic based on consecutively retrieved errors).
	// It returns a boolean indicating whether or not the loop should continue.
	HandleError(ctx context.Context, frameErr error) bool
}

type defaultErrorHandler struct{}

func (eH *defaultErrorHandler) HandleError(ctx context.Context, frameErr error) bool {
	if frameErr != nil {
		Logger.Debugw("error getting frame", "error", frameErr)
		return true
	}
	return false
}

// streamSource will stream a source of images forever to the stream until the given context tells it to cancel.
func streamSource(ctx context.Context, once func(), is ImageSource, stream Stream, errHandler ErrorHandler) {
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
		if errHandler.HandleError(ctx, err) {
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
	streamSource(ctx, nil, is, stream, &defaultErrorHandler{})
}

// StreamSourceWithErrorHandler streams the given image source to the stream forever
// until context signals cancellation, frame errors are sent via the error handler
func StreamSourceWithErrorHandler(
	ctx context.Context, is ImageSource, stream Stream, errHandler ErrorHandler,
) {
	streamSource(ctx, nil, is, stream, errHandler)
}
