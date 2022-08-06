package gostream

import (
	"context"
	"image"
)

// An ImageSource is responsible for producing images when requested. A source
// should produce the image as quickly as possible and introduce no rate limiting
// of its own as that is handled internally.
type ImageSource interface {
	// Next returns an image along with a function to release
	// the image once it is no longer used. Not calling the function
	// will not leak memory but may cause the implementer to not be
	// as efficient with memory.
	Next(ctx context.Context) (image.Image, func(), error)
}

// An ImageSourceFunc is a helper to turn a function into an ImageSource.
type ImageSourceFunc func(ctx context.Context) (image.Image, func(), error)

// Next calls the underlying function to get an image.
func (isf ImageSourceFunc) Next(ctx context.Context) (image.Image, func(), error) {
	return isf(ctx)
}
