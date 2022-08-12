package utils

import (
	"context"

	"github.com/edaniels/gostream"
	"github.com/edaniels/gostream/media"
)

// ErrorHandler receives the error returned by a TSource.Next
// regardless of whether or not the error is nil (This allows
// for error handling logic based on consecutively retrieved errors).
// It returns a boolean indicating whether or not the loop should continue.
type ErrorHandler func(ctx context.Context, frameErr error) bool

// streamImageSource will stream a source of images forever to the stream until the given context tells it to cancel.
func streamImageSource(ctx context.Context, once func(), is gostream.ImageSource, stream gostream.Stream, errHandler ErrorHandler) error {
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
		case inputFrames <- gostream.FrameReleasePair{frame, release}:
		}
	}
}

// streamAudioSource will stream a source of audio chunks forever to the stream until the given context tells it to cancel.
func streamAudioSource(ctx context.Context, once func(), as media.AudioSource, stream gostream.Stream, errHandler ErrorHandler) error {
	if once != nil {
		once()
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-stream.StreamingReady():
	}
	props, err := as.Properties(ctx)
	if err != nil {
		return err
	}
	inputChunks, err := stream.InputAudioChunks(props.Latency)
	if err != nil {
		return err
	}
	audioStream, err := as.Stream(ctx)
	if err != nil {
		return err
	}
	defer audioStream.Close()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		chunk, release, err := audioStream.Next(ctx)
		// if errHandler returns true, it means DO NOT continue with the
		// the rest of the logic on the current iteration
		if errHandler(ctx, err) {
			continue
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case inputChunks <- gostream.AudioChunkReleasePair{
			Chunk:   chunk,
			Release: release,
		}:
		}
	}
}

// StreamImageSource streams the given image source to the stream forever until context signals cancellation.
func StreamImageSource(ctx context.Context, is gostream.ImageSource, stream gostream.Stream) error {
	return streamImageSource(ctx, nil, is, stream, func(ctx context.Context, frameErr error) bool {
		if frameErr != nil {
			gostream.Logger.Debugw("error getting frame", "error", frameErr)
			return true
		}
		return false
	})
}

// StreamAudioSource streams the given image source to the stream forever until context signals cancellation.
func StreamAudioSource(ctx context.Context, as media.AudioSource, stream gostream.Stream) error {
	return streamAudioSource(ctx, nil, as, stream, func(ctx context.Context, frameErr error) bool {
		if frameErr != nil {
			gostream.Logger.Debugw("error getting frame", "error", frameErr)
			return true
		}
		return false
	})
}

// StreamImageSourceWithErrorHandler streams the given image source to the stream forever
// until context signals cancellation, frame errors are sent via the error handler.
func StreamImageSourceWithErrorHandler(
	ctx context.Context, is gostream.ImageSource, stream gostream.Stream, errHandler ErrorHandler,
) error {
	return streamImageSource(ctx, nil, is, stream, errHandler)
}

// StreamAudioSourceWithErrorHandler streams the given audio source to the stream forever
// until context signals cancellation, audio errors are sent via the error handler.
func StreamAudioSourceWithErrorHandler(
	ctx context.Context, as media.AudioSource, stream gostream.Stream, errHandler ErrorHandler,
) error {
	return streamAudioSource(ctx, nil, as, stream, errHandler)
}
