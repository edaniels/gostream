package gostream

import (
	"github.com/edaniels/golog"

	"github.com/edaniels/gostream/codec"
)

// A StreamConfig describes how a Stream should be managed.
type StreamConfig struct {
	Name           string
	EncoderFactory codec.EncoderFactory
	// TargetFrameRate will hint to the stream to try to maintain this frame rate.
	TargetFrameRate int
	Logger          golog.Logger
}
