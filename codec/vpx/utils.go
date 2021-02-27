package vpx

import (
	"fmt"

	"github.com/edaniels/golog"
	"github.com/edaniels/gostream"
)

var DefaultRemoteViewConfig = gostream.PartialDefaultRemoteViewConfig

func init() {
	DefaultRemoteViewConfig.EncoderFactory = NewEncoderFactory(CodecVP8, false)
}

func NewEncoderFactory(codec VCodec, debug bool) gostream.EncoderFactory {
	return &factory{codec, debug}
}

type factory struct {
	codec VCodec
	debug bool
}

func (f *factory) New(width, height int, logger golog.Logger) (gostream.Encoder, error) {
	return NewEncoder(f.codec, width, height, f.debug, logger)
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
