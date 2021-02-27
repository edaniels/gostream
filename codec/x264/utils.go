package x264

import (
	"github.com/edaniels/golog"
	"github.com/edaniels/gostream"
)

var DefaultRemoteViewConfig = gostream.PartialDefaultRemoteViewConfig

func init() {
	DefaultRemoteViewConfig.EncoderFactory = NewEncoderFactory(false)
}

func NewEncoderFactory(debug bool) gostream.EncoderFactory {
	return &factory{debug}
}

type factory struct {
	debug bool
}

func (f *factory) New(width, height int, logger golog.Logger) (gostream.Encoder, error) {
	return NewEncoder(width, height, f.debug, logger)
}

func (f *factory) MIMEType() string {
	return "video/H264"
}
