package mmal

import (
	"github.com/edaniels/golog"
	"github.com/edaniels/gostream"
)

// DefaultViewConfig configures MMAL as the encoder for a view.
var DefaultViewConfig = gostream.PartialDefaultViewConfig

func init() {
	DefaultViewConfig.EncoderFactory = NewEncoderFactory(false)
}

// NewEncoderFactory returns an MMAL encoder factory.
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
