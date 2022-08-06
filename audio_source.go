package gostream

import (
	"context"

	"github.com/pion/mediadevices/pkg/wave"
)

// An AudioSource is responsible for producing audio chunks when requested. A source
// should produce the chunk as quickly as possible and introduce no rate limiting
// of its own as that is handled internally.
type AudioSource interface {
	// Next returns a chunk along with a function to release
	// the data once it is no longer used. Not calling the function
	// will not leak memory but may cause the implementer to not be
	// as efficient with memory.
	Next(ctx context.Context) (wave.Audio, func(), error)
}

// An AudioSoruceFunc is a helper to turn a function into an AudioSoruce.
type AudioSoruceFunc func(ctx context.Context) (wave.Audio, func(), error)

// Next calls the underlying function to get an image.
func (asf AudioSoruceFunc) Next(ctx context.Context) (wave.Audio, func(), error) {
	return asf(ctx)
}
