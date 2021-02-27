package x264

import (
	"image"

	"github.com/edaniels/gostream"

	"github.com/edaniels/golog"
	"github.com/pion/mediadevices/pkg/codec"
	"github.com/pion/mediadevices/pkg/codec/x264"
	"github.com/pion/mediadevices/pkg/prop"
)

type encoder struct {
	codec  codec.ReadCloser
	img    image.Image
	debug  bool
	logger golog.Logger
}

const bitrate = 3_200_000

func NewEncoder(width, height, keyFrameInterval int, debug bool, logger golog.Logger) (gostream.Encoder, error) {
	enc := &encoder{debug: debug, logger: logger}

	var builder codec.VideoEncoderBuilder
	params, err := x264.NewParams()
	if err != nil {
		return nil, err
	}
	builder = &params
	params.BitRate = bitrate
	params.KeyFrameInterval = keyFrameInterval

	codec, err := builder.BuildVideoEncoder(enc, prop.Media{
		Video: prop.Video{
			Width:  width,
			Height: height,
		},
	})
	if err != nil {
		return nil, err
	}
	enc.codec = codec

	return enc, nil
}

func (v *encoder) Read() (img image.Image, release func(), err error) {
	return v.img, nil, nil
}

func (v *encoder) Encode(img image.Image) ([]byte, error) {
	v.img = img
	data, release, err := v.codec.Read()
	dataCopy := make([]byte, len(data))
	copy(dataCopy, data)
	release()
	return dataCopy, err
}
