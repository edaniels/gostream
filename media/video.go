package media

import (
	"image"

	"github.com/pion/mediadevices/pkg/driver"
	"github.com/pion/mediadevices/pkg/io/video"
)

// NewVideoReadCloser instantiates a new video read closer and references the given
// driver.
func NewVideoReadCloser(d driver.Driver, r video.Reader) ReadCloser[image.Image] {
	return newReadCloser[image.Image](d, r)
}
