package codec

import (
	"github.com/edaniels/golog"
	"github.com/pion/mediadevices/pkg/wave"
)

// An AudioEncoder is anything that can encode audo chunks into bytes. This means that
// the encoder must follow some type of format dictated by a type (see AudioEncoderFactory.MimeType).
// An encoder that produces bytes of different encoding formats per call is invalid.
type AudioEncoder interface {
	Encode(chunk wave.Audio) ([]byte, error)
}

// An EncoderFactory produces Encoders and provides information about the underlying encoder itself.
type AudioEncoderFactory interface {
	New(sampleRate, channelCount int, logger golog.Logger) (AudioEncoder, error)
	MIMEType() string
}
