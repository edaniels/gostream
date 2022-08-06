package gostream

import (
	"context"
)

// ErrorHandler receives the error returned by a TSource.Next
// regardless of whether or not the error is nil (This allows
// for error handling logic based on consecutively retrieved errors).
// It returns a boolean indicating whether or not the loop should continue.
type ErrorHandler func(ctx context.Context, frameErr error) bool

// streamImageSource will stream a source of images forever to the stream until the given context tells it to cancel.
func streamImageSource(ctx context.Context, once func(), is ImageSource, stream Stream, errHandler ErrorHandler) error {
	if once != nil {
		once()
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-stream.StreamingReady():
	}
	inputFrames, err := stream.InputImageFrames()
	if err != nil {
		return err
	}
	for {
		select {
		case <-ctx.Done():
			return err
		default:
		}
		frame, release, err := is.Next(ctx)
		// if errHandler returns true, it means DO NOT continue with the
		// the rest of the logic on the current iteration
		if errHandler(ctx, err) {
			continue
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case inputFrames <- FrameReleasePair{frame, release}:
		}
	}
}

// streamAudioSource will stream a source of audio chunks forever to the stream until the given context tells it to cancel.
func streamAudioSource(ctx context.Context, once func(), as AudioSource, stream Stream, errHandler ErrorHandler) error {
	if once != nil {
		once()
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-stream.StreamingReady():
	}
	inputChunks, err := stream.InputAudioChunks()
	if err != nil {
		return err
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		chunk, release, err := as.Next(ctx)
		// if errHandler returns true, it means DO NOT continue with the
		// the rest of the logic on the current iteration
		if errHandler(ctx, err) {
			continue
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case inputChunks <- AudioChunkReleasePair{chunk, release}:
		}
	}
}

// StreamImageSource streams the given image source to the stream forever until context signals cancellation.
func StreamImageSource(ctx context.Context, is ImageSource, stream Stream) error {
	return streamImageSource(ctx, nil, is, stream, func(ctx context.Context, frameErr error) bool {
		if frameErr != nil {
			Logger.Debugw("error getting frame", "error", frameErr)
			return true
		}
		return false
	})
}

// StreamAudioSource streams the given image source to the stream forever until context signals cancellation.
func StreamAudioSource(ctx context.Context, as AudioSource, stream Stream) error {
	return streamAudioSource(ctx, nil, as, stream, func(ctx context.Context, frameErr error) bool {
		if frameErr != nil {
			Logger.Debugw("error getting frame", "error", frameErr)
			return true
		}
		return false
	})
}

// StreamImageSourceWithErrorHandler streams the given image source to the stream forever
// until context signals cancellation, frame errors are sent via the error handler.
func StreamImageSourceWithErrorHandler(
	ctx context.Context, is ImageSource, stream Stream, errHandler ErrorHandler,
) error {
	return streamImageSource(ctx, nil, is, stream, errHandler)
}

// StreamAudioSourceWithErrorHandler streams the given audio source to the stream forever
// until context signals cancellation, audio errors are sent via the error handler.
func StreamAudioSourceWithErrorHandler(
	ctx context.Context, as AudioSource, stream Stream, errHandler ErrorHandler,
) error {
	return streamAudioSource(ctx, nil, as, stream, errHandler)
}
