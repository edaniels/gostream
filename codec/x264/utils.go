package x264

import (
	"github.com/trevor403/gostream"
	"github.com/trevor403/gostream/codec"

	"github.com/edaniels/golog"
)

// DefaultViewConfig configures x264 as the encoder for a view.
var DefaultViewConfig = gostream.PartialDefaultViewConfig

func init() {
	DefaultViewConfig.EncoderFactory = NewEncoderFactory()
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
