package gostream

import (
	"github.com/edaniels/golog"

	"github.com/edaniels/gostream/codec"
)

// A StreamConfig describes how a Stream should be managed.
type StreamConfig struct {
	Name                string
	EncoderFactory      codec.EncoderFactory
	AudioEncoderFactory codec.AudioEncoderFactory

	// TargetFrameRate will hint to the stream to try to maintain this frame rate.
	TargetFrameRate int

	// TODO(erd): Is there some kind of target audio "rate" or... no?
	// TOOD(erd): difference between sample rate and clock rate

	Logger golog.Logger
}
