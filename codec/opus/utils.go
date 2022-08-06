package x264

import (
	"github.com/edaniels/golog"

	"github.com/edaniels/gostream"
	"github.com/edaniels/gostream/codec"
)

// DefaultStreamConfig configures Opus as the audio encoder for a stream.
var DefaultStreamConfig gostream.StreamConfig

func init() {
	DefaultStreamConfig.AudioEncoderFactory = NewAudioEncoderFactory()
}

// NewAudioEncoderFactory returns an Opus audio encoder factory.
func NewAudioEncoderFactory() codec.AudioEncoderFactory {
	return &factory{}
}

type factory struct{}

func (f *factory) New(ogger golog.Logger) (codec.AudioEncoder, error) {
	return NewAudioEncoder(logger)
}

func (f *factory) MIMEType() string {
	return "audio/opus"
}
