package gostream

import (
	"context"

	"go.viam.com/utils"
)

// StreamVideoSource streams the given video source to the stream forever until context signals cancellation.
func StreamVideoSource(ctx context.Context, vs VideoSource, stream Stream) error {
	return streamMediaSource(ctx, nil, vs, stream, func(ctx context.Context, frameErr error) {
		Logger.Debugw("error getting frame", "error", frameErr)
	}, stream.InputVideoFrames)
}

// StreamAudioSource streams the given video source to the stream forever until context signals cancellation.
func StreamAudioSource(ctx context.Context, as AudioSource, stream Stream) error {
	return streamMediaSource(ctx, nil, as, stream, func(ctx context.Context, frameErr error) {
		Logger.Debugw("error getting frame", "error", frameErr)
	}, stream.InputAudioChunks)
}

// StreamVideoSourceWithErrorHandler streams the given video source to the stream forever
// until context signals cancellation, frame errors are sent via the error handler.
func StreamVideoSourceWithErrorHandler(
	ctx context.Context, vs VideoSource, stream Stream, errHandler ErrorHandler,
) error {
	return streamMediaSource(ctx, nil, vs, stream, errHandler, stream.InputVideoFrames)
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
	mediaStream, err := ms.Stream(ctx, errHandler)
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
		if err != nil {
			continue
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case input <- MediaReleasePair[T]{media, release}:
		}
	}
}
