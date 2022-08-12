package media

import (
	"image"

	"github.com/pion/mediadevices/pkg/driver"
	"github.com/pion/mediadevices/pkg/io/video"
	"github.com/pion/mediadevices/pkg/prop"
)

// NewVideoSource instantiates a new video source.
func NewVideoSource(r video.Reader, p prop.Video) Source[image.Image, prop.Video] {
	return newSource[image.Image](nil, r, p, imageCopy)
}

// NewVideoSourceForDriver instantiates a new video source and references the given driver.
func NewVideoSourceForDriver(d driver.Driver, r video.Reader, p prop.Video) Source[image.Image, prop.Video] {
	return newSource[image.Image](d, r, p, imageCopy)
}

func imageCopy(src image.Image) image.Image {
	buffer := video.NewFrameBuffer(0)
	realSrc, _ := src.(image.Image)
	buffer.StoreCopy(realSrc)
	return buffer.Load()
}
