package vpx

import (
	"fmt"

	"github.com/trevor403/gostream"
	"github.com/trevor403/gostream/codec"

	"github.com/edaniels/golog"
)

// DefaultViewConfig configures vpx as the encoder for a view.
var DefaultViewConfig = gostream.PartialDefaultViewConfig

func init() {
	DefaultViewConfig.EncoderFactory = NewEncoderFactory(Version8)
}

// NewEncoderFactory returns a vpx factory for the given vpx codec.
func NewEncoderFactory(codecVersion Version) codec.EncoderFactory {
	return &factory{codecVersion}
}

type factory struct {
	codecVersion Version
}

func (f *factory) New(width, height, keyFrameInterval int, logger golog.Logger) (codec.Encoder, error) {
	return NewEncoder(f.codecVersion, width, height, keyFrameInterval, logger)
}

func (f *factory) MIMEType() string {
	switch f.codecVersion {
	case Version8:
		return "video/vp8"
	case Version9:
		return "video/vp9"
	default:
		panic(fmt.Errorf("unknown codec version %q", f.codecVersion))
	}
}
