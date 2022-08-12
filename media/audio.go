package media

import (
	"context"

	"github.com/pion/mediadevices/pkg/driver"
	"github.com/pion/mediadevices/pkg/io/audio"
	"github.com/pion/mediadevices/pkg/prop"
	"github.com/pion/mediadevices/pkg/wave"
)

// An AudioSource is responsible for producing audio chunks when requested. A source
// should produce the chunk as quickly as possible and introduce no rate limiting
// of its own as that is handled internally.
type AudioSource interface {
	// Stream returns a stream of chunks until closed.
	// This is preferred to be used in low latency, high quality scenarios (e.g. VOIP).
	Stream(ctx context.Context) (Stream[wave.Audio], error)

	// Properties returns information about the audio that will be produced.
	Properties(ctx context.Context) (prop.Audio, error)

	// Close closes the media source and waits for all active operations to terminate.
	Close(ctx context.Context) error
}

// An AudioStream is similar to an AudioSource but is used to represent a continuous,
// possibly buffered stream of data.
type AudioStream interface {
	// Next returns a chunk along with a function to release
	// the data once it is no longer used. Not calling the function
	// will not leak memory but may cause the implementer to not be
	// as efficient with memory.
	Next(ctx context.Context) (wave.Audio, func(), error)

	// Close terminates the stream.
	Close()
}

func audioCopy(src wave.Audio) wave.Audio {
	buffer := wave.NewBuffer()
	realSrc, _ := src.(wave.Audio)
	buffer.StoreCopy(realSrc)
	return buffer.Load()
}

// NewAudioSource instantiates a new audio read closer.
func NewAudioSource(r audio.Reader, p prop.Audio) Source[wave.Audio, prop.Audio] {
	return newSource[wave.Audio](nil, r, p, audioCopy)
}

// NewAudioSourceForDriver instantiates a new audio read closer and references the given
// driver.
func NewAudioSourceForDriver(d driver.Driver, r audio.Reader, p prop.Audio) Source[wave.Audio, prop.Audio] {
	return newSource[wave.Audio](d, r, p, audioCopy)
}
