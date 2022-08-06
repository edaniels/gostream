// Package opus contains the opus video codec.
package opus

import (
	"github.com/edaniels/golog"
	"github.com/pion/mediadevices/pkg/codec"
	"github.com/pion/mediadevices/pkg/codec/opus"
	"github.com/pion/mediadevices/pkg/prop"
	"github.com/pion/mediadevices/pkg/wave"

	ourcodec "github.com/edaniels/gostream/codec"
)

type encoder struct {
	codec  codec.ReadCloser
	chunk  wave.Audio
	logger golog.Logger
}

// Gives suitable results. Probably want to make this configurable this in the future.
const bitrate = 32000

// NewEncoder returns an MMAL encoder that can encode images of the given width and height. It will
// also ensure that it produces key frames at the given interval.
func NewAudioEncoder(sampleRate, channelCount int, logger golog.Logger) (ourcodec.AudioEncoder, error) {
	enc := &encoder{logger: logger}

	var builder codec.AudioEncoderBuilder
	params, err := opus.NewParams()
	if err != nil {
		return nil, err
	}
	builder = &params
	params.BitRate = bitrate

	codec, err := builder.BuildAudioEncoder(enc, prop.Media{
		Audio: prop.Audio{
			SampleRate:   sampleRate,
			ChannelCount: channelCount,
		},
	})
	if err != nil {
		return nil, err
	}
	enc.codec = codec

	return enc, nil
}

// Read returns an audio chunk for codec to process.
func (a *encoder) Read() (chunk wave.Audio, release func(), err error) {
	return a.chunk, nil, nil
}

// Encode asks the codec to process the given audio chunk.
func (a *encoder) Encode(chunk wave.Audio) ([]byte, error) {
	a.chunk = chunk
	data, release, err := a.codec.Read()
	dataCopy := make([]byte, len(data))
	copy(dataCopy, data)
	release()
	return dataCopy, err
}
