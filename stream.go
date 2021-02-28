package gostream

import "image"

// A Stream is sink that accepts any image frames for the purpose
// of displaying in a view.
type Stream interface {
	InputFrames() chan<- image.Image
}
