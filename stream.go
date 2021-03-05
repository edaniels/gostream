package gostream

import "image"

// A Stream is sink that accepts any image frames for the purpose
// of displaying in a view.
type Stream interface {
	InputFrames() chan<- FrameReleasePair
}

// FrameReleasePair associates a frame with a corresponding
// function to release its resources once the receiver of a
// pair is finished with the frame.
type FrameReleasePair struct {
	Frame   image.Image
	Release func()
}
