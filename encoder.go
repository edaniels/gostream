package gostream

import (
	"image"

	"github.com/edaniels/golog"
)

type Encoder interface {
	Encode(img image.Image) ([]byte, error)
}

type EncoderFactory interface {
	New(height, width, keyFrameInterval int, logger golog.Logger) (Encoder, error)
	MIMEType() string
}
