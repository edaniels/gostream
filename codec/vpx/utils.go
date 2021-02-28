package vpx

import (
	"fmt"

	"github.com/edaniels/gostream"
	"github.com/edaniels/gostream/codec"

	"github.com/edaniels/golog"
)

// DefaultViewConfig configures vpx as the encoder for a view.
var DefaultViewConfig = gostream.PartialDefaultViewConfig

func init() {
	DefaultViewConfig.EncoderFactory = NewEncoderFactory(CodecVP8)
}

// NewEncoderFactory returns a vpx factory for the given vpx codec.
func NewEncoderFactory(codec VCodec) codec.EncoderFactory {
	return &factory{codec}
}

type factory struct {
	codec VCodec
}

func (f *factory) New(width, height, keyFrameInterval int, logger golog.Logger) (codec.Encoder, error) {
	return NewEncoder(f.codec, width, height, keyFrameInterval, logger)
}

func (f *factory) MIMEType() string {
	switch f.codec {
	case CodecVP8:
		return "video/vp8"
	case CodecVP9:
		return "video/vp9"
	default:
		panic(fmt.Errorf("unknown codec %q", f.codec))
	}
}
