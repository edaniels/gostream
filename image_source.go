package gostream

import (
	"context"
	"image"
)

// An ImageSource is responsible for producing images when requested. A source
// should produce the image as quickly as possible and introduce no rate limiting
// of its own as that is handled internally.
type ImageSource interface {
	Next(ctx context.Context) (image.Image, error)
	Close() error
}

// An ImageSourceFunc is a helper to turn a function into an ImageSource
type ImageSourceFunc func(ctx context.Context) (image.Image, error)

func (isf ImageSourceFunc) Next(ctx context.Context) (image.Image, error) {
	return isf(ctx)
}

func (isf ImageSourceFunc) Close() error {
	return nil
}
