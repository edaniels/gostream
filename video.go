package gostream

import (
	"image"

	"github.com/pion/mediadevices/pkg/driver"
	"github.com/pion/mediadevices/pkg/io/video"
	"github.com/pion/mediadevices/pkg/prop"
)

// A VideoSource is responsible for producing images when requested. A source
// should produce the image as quickly as possible and introduce no rate limiting
// of its own as that is handled internally.
type VideoSource = MediaSource[image.Image, prop.Video]

// NewVideoSource instantiates a new video source.
func NewVideoSource(r MediaReader[image.Image], p prop.Video) MediaSource[image.Image, prop.Video] {
	return newMediaSource(nil, r, p, imageCopy)
}

// NewVideoSourceForDriver instantiates a new video source and references the given driver.
func NewVideoSourceForDriver(d driver.Driver, r MediaReader[image.Image], p prop.Video) MediaSource[image.Image, prop.Video] {
	return newMediaSource(d, r, p, imageCopy)
}

func imageCopy(src image.Image) image.Image {
	buffer := video.NewFrameBuffer(0)
	buffer.StoreCopy(src)
	return buffer.Load()
}
