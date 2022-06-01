package gostream

import (
	"context"
	"errors"
	"math"
	"time"
)

// maxErrorSleepSec sets a maximum sleep time to the exponential backoff
// determined by sleepTimeFromErrorCount
var maxErrorSleepSec = math.Pow10(9) * 2 // two seconds
const maxSleepAttempts = 20

func sleepTimeFromErrorCount(errCount int) int {
	expBackoffMillisec := math.Pow(6.0, float64(errCount))
	expBackoffNanosec := expBackoffMillisec * math.Pow10(6)
	return int(math.Min(expBackoffNanosec, maxErrorSleepSec))
}

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
			Logger.Debugw("error getting frame", "error", err)
			if prevErr == nil {
				prevErr = err
			} else if errors.Is(prevErr, err) {
				errorCount += 1
			} else {
				errorCount = 0
			}
			if errorCount > 0 {
				errorCount = int(math.Min(float64(errorCount), float64(maxSleepAttempts)))
				dur := sleepTimeFromErrorCount(errorCount)
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
func StreamSource(ctx context.Context, is ImageSource, stream Stream) {
	streamSource(ctx, nil, is, stream)
}
