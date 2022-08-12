package gostream

import (
	"context"

	"go.viam.com/utils"
)

// ErrorHandler receives the error returned by a TSource.Next
// regardless of whether or not the error is nil (This allows
// for error handling logic based on consecutively retrieved errors).
// It returns a boolean indicating whether or not the loop should continue.
type ErrorHandler func(ctx context.Context, frameErr error) bool

// StreamImageSource streams the given image source to the stream forever until context signals cancellation.
func StreamImageSource(ctx context.Context, is ImageSource, stream Stream) error {
	return streamMediaSource(ctx, nil, is, stream, func(ctx context.Context, frameErr error) bool {
		if frameErr != nil {
			Logger.Debugw("error getting frame", "error", frameErr)
			return true
		}
		return false
	}, stream.InputImageFrames)
}

// StreamAudioSource streams the given image source to the stream forever until context signals cancellation.
func StreamAudioSource(ctx context.Context, as AudioSource, stream Stream) error {
	return streamMediaSource(ctx, nil, as, stream, func(ctx context.Context, frameErr error) bool {
		if frameErr != nil {
			Logger.Debugw("error getting frame", "error", frameErr)
			return true
		}
		return false
	}, stream.InputAudioChunks)
}

// StreamImageSourceWithErrorHandler streams the given image source to the stream forever
// until context signals cancellation, frame errors are sent via the error handler.
func StreamImageSourceWithErrorHandler(
	ctx context.Context, is ImageSource, stream Stream, errHandler ErrorHandler,
) error {
	return streamMediaSource(ctx, nil, is, stream, errHandler, stream.InputImageFrames)
}

// StreamAudioSourceWithErrorHandler streams the given audio source to the stream forever
// until context signals cancellation, audio errors are sent via the error handler.
func StreamAudioSourceWithErrorHandler(
	ctx context.Context, as AudioSource, stream Stream, errHandler ErrorHandler,
) error {
	return streamMediaSource(ctx, nil, as, stream, errHandler, stream.InputAudioChunks)
}

// streamMediaSource will stream a source of media forever to the stream until the given context tells it to cancel.
func streamMediaSource[T any, U any](
	ctx context.Context,
	once func(),
	ms MediaSource[T, U],
	stream Stream,
	errHandler ErrorHandler,
	inputChan func(props U) (chan<- MediaReleasePair[T], error),
) error {
	if once != nil {
		once()
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-stream.StreamingReady():
	}
	props, err := ms.Properties(ctx)
	if err != nil {
		return err
	}
	input, err := inputChan(props)
	if err != nil {
		return err
	}
	mediaStream, err := ms.Stream(ctx)
	if err != nil {
		return err
	}
	defer func() {
		utils.UncheckedError(mediaStream.Close(ctx))
	}()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		media, release, err := mediaStream.Next(ctx)
		// if errHandler returns true, it means DO NOT continue with the
		// the rest of the logic on the current iteration
		if errHandler(ctx, err) {
			continue
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case input <- MediaReleasePair[T]{media, release}:
		}
	}
}
