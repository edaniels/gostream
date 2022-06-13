package gostream

import (
	"context"
	"errors"
	"math"
	"time"

	"go.viam.com/utils"
)

// BackoffTuningOptions represents a set of parameters for determining
// exponential backoff when receiving multiple simultaneous errors. This is
// to reduce the number of errors logged in the case of minor dicontinuity
// in the camera stream.
// The number of milliseconds slept at a particular attempt i is determined by
// min(ExpBase^(i) + Offset, MaxSleepMilliSec)
type BackoffTuningOptions struct {
	// ExpBase is a tuning parameter for backoff used as described above
	ExpBase float64
	// Offset is a tuning parameter for backoff used as described above
	Offset float64
	// MaxSleepMilliSec determines the maximum amount of time that streamSource is
	// permitted to a sleep after receiving a single error
	MaxSleepMilliSec float64
	// MaxSleepAttempts determines the number of consecutive errors for which
	// streamSource will sleep
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
				// time.Sleep(time.Duration(dur))
				utils.SelectContextOrWait(ctx, time.Duration(dur))
			} else {
				panic(err)
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

// StreamSourceWithOptions streams the given image source to the stream forever until context signals cancellation.
func StreamSourceWithOptions(
	ctx context.Context, is ImageSource, stream Stream,
	backoffOpts *BackoffTuningOptions,
) {
	streamSource(ctx, nil, is, stream, backoffOpts)
}

// StreamSource is deprecated in favor of StreamSourceWithOptions
func StreamSource(ctx context.Context, is ImageSource, stream Stream) {
	backoffOpts := &BackoffTuningOptions{
		ExpBase:          6.0,
		Offset:           0,
		MaxSleepMilliSec: math.Pow10(6) * 2, // two seconds
		MaxSleepAttempts: 20,
	}
	streamSource(ctx, nil, is, stream, backoffOpts)
}
