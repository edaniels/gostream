package media

import (
	"image"

	"github.com/pion/mediadevices/pkg/driver"
	"github.com/pion/mediadevices/pkg/io/video"
	"github.com/pion/mediadevices/pkg/prop"
)

// NewVideoReadCloser instantiates a new video read closer and references the given
// driver.
func NewVideoReadCloser(d driver.Driver, r video.Reader, p prop.Video) ReadCloser[image.Image, prop.Video] {
	return newReadCloser[image.Image](d, r, p)
}
