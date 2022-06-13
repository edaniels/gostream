package gostream

import (
	"context"
	"errors"
	"math"
	"time"
)

type BackoffTuningOptions struct {
	ExpBase          float64
	Offset           float64
	MaxSleepMilliSec float64
	MaxSleepAttempts int
}

func (opts *BackoffTuningOptions) GetSleepTimeFromErrorCount(errCount int) int {
	expBackoffMillisec := math.Pow(opts.ExpBase, float64(errCount)) + opts.Offset
	expBackoffNanosec := expBackoffMillisec * math.Pow10(6)
	maxSleepNanosec := opts.MaxSleepMilliSec * math.Pow10(6)
	return int(math.Min(expBackoffNanosec, maxSleepNanosec))
}

// streamSource will stream a source of images forever to the stream until the given context tells it to cancel.
func streamSource(
	ctx context.Context,
	once func(),
	is ImageSource,
	stream Stream,
	backoffOpts *BackoffTuningOptions,
) {
	if once != nil {
		once()
	}
	select {
	case <-ctx.Done():
		return
	case <-stream.StreamingReady():
	}
	var prevErr error
	errorCount := 0
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		frame, release, err := is.Next(ctx)
		if err != nil {
			if prevErr == nil {
				prevErr = err
			} else if errors.Is(prevErr, err) {
				errorCount += 1
			} else {
				errorCount = 0
			}
			canSleep := (errorCount > 0) && (errorCount < backoffOpts.MaxSleepAttempts)
			if canSleep && (backoffOpts != nil) {
				Logger.Debugw("error getting frame", "error", err)
				dur := backoffOpts.GetSleepTimeFromErrorCount(errorCount)
				time.Sleep(time.Duration(dur))
			}
			continue
		} else {
			errorCount = 0
		}
		select {
		case <-ctx.Done():
			return
		case stream.InputFrames() <- FrameReleasePair{frame, release}:
		}
	}
}

// StreamSource streams the given image source to the stream forever until context signals cancellation.
func StreamSource(
	ctx context.Context, is ImageSource, stream Stream,
	backoffOpts *BackoffTuningOptions,
) {
	streamSource(ctx, nil, is, stream, backoffOpts)
}
