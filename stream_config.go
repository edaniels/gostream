package gostream

import (
	"time"

	"github.com/edaniels/golog"

	"github.com/edaniels/gostream/codec"
)

// A StreamConfig describes how a Stream should be managed.
type StreamConfig struct {
	Name                string
	VideoEncoderFactory codec.VideoEncoderFactory
	AudioEncoderFactory codec.AudioEncoderFactory

	// TargetFrameRate will hint to the stream to try to maintain this frame rate.
	TargetFrameRate int

	// AudioLatency specifies how long in between audio samples.
	AudioLatency time.Duration

	Logger golog.Logger
}
