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
	// Next returns a chunk along with a function to release
	// the data once it is no longer used. Not calling the function
	// will not leak memory but may cause the implementer to not be
	// as efficient with memory. The following call will not try to guarantee
	// that the next chunk is the following one in the sequence.
	Next(ctx context.Context) (wave.Audio, func(), error)

	// Stream returns a stream of chunks until closed.
	// This is preferred to be used in low latency, high quality scenarios (e.g. VOIP).
	Stream(ctx context.Context) (Stream[wave.Audio], error)

	// Properties returns information about the audio that will be produced.
	Properties(ctx context.Context) (prop.Audio, error)
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

// NewAudioReadCloser instantiates a new audio read closer and references the given
// driver.
func NewAudioReadCloser(d driver.Driver, r audio.Reader, p prop.Audio) ReadCloser[wave.Audio, prop.Audio] {
	return newReadCloser[wave.Audio](d, r, p)
}
