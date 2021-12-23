package x264

import (
	"github.com/edaniels/golog"

	"github.com/edaniels/gostream"
	"github.com/edaniels/gostream/codec"
)

// DefaultStreamConfig configures x264 as the encoder for a stream.
var DefaultStreamConfig gostream.StreamConfig

func init() {
	DefaultStreamConfig.EncoderFactory = NewEncoderFactory()
}

// NewEncoderFactory returns an x264 encoder factory.
func NewEncoderFactory() codec.EncoderFactory {
	return &factory{}
}

type factory struct{}

func (f *factory) New(width, height, keyFrameInterval int, logger golog.Logger) (codec.Encoder, error) {
	return NewEncoder(width, height, keyFrameInterval, logger)
}

func (f *factory) MIMEType() string {
	return "video/H264"
}
