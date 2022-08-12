package gostream

import (
	"github.com/pion/mediadevices/pkg/driver"
	"github.com/pion/mediadevices/pkg/io/audio"
	"github.com/pion/mediadevices/pkg/prop"
	"github.com/pion/mediadevices/pkg/wave"
)

// An AudioSource is responsible for producing audio chunks when requested. A source
// should produce the chunk as quickly as possible and introduce no rate limiting
// of its own as that is handled internally.
type AudioSource = MediaSource[wave.Audio, prop.Audio]

// // An ImageSourceFunc is a helper to turn a function into an ImageSource.
// type ImageSourceFunc func(ctx context.Context) (image.Image, func(), error)

// // Next calls the underlying function to get an image.
// func (isf ImageSourceFunc) Next(ctx context.Context) (image.Image, func(), error) {
// 	return isf(ctx)
// }

func audioCopy(src wave.Audio) wave.Audio {
	buffer := wave.NewBuffer()
	buffer.StoreCopy(src)
	return buffer.Load()
}

// NewAudioSource instantiates a new audio read closer.
func NewAudioSource(r audio.Reader, p prop.Audio) MediaSource[wave.Audio, prop.Audio] {
	return newMediaSource[wave.Audio](nil, r, p, audioCopy)
}

// NewAudioSourceForDriver instantiates a new audio read closer and references the given
// driver.
func NewAudioSourceForDriver(d driver.Driver, r audio.Reader, p prop.Audio) MediaSource[wave.Audio, prop.Audio] {
	return newMediaSource[wave.Audio](d, r, p, audioCopy)
}
