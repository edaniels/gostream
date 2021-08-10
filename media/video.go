package media

import (
	"context"
	"image"

	"github.com/trevor403/mediadevices/pkg/driver"
	"github.com/trevor403/mediadevices/pkg/io/video"
)

// A VideoReadCloser is a video.Reader that requires it be closed.
type VideoReadCloser interface {
	video.Reader
	// Next returns the next image read. This method satisfies APIs that use Next instead
	// of Read with a given context. The release function should be called once the
	// image no longer will be utilized.
	Next(ctx context.Context) (image.Image, func(), error)
	// Close cleans up any associated resources with the video.Reader (e.g. a Driver).
	Close() error
}

type videoReadCloser struct {
	videoDriver driver.Driver
	videoReader video.Reader
}

func (vrc videoReadCloser) Read() (image.Image, func(), error) {
	return vrc.videoReader.Read()
}

func (vrc videoReadCloser) Next(ctx context.Context) (image.Image, func(), error) {
	return vrc.Read()
}

func (vrc videoReadCloser) Close() error {
	return vrc.videoDriver.Close()
}
