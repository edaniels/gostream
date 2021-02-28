package mediadevices

import (
	"context"
	"image"
	"image/draw"

	"github.com/pion/mediadevices/pkg/driver"
	"github.com/pion/mediadevices/pkg/io/video"
)

// A VideoReadCloser is a video.Reader that requires it be closed.
type VideoReadCloser interface {
	video.Reader
	// Close cleans up any associated resources with the video.Reader (e.g. a Driver).
	Close() error
}

type videoReadCloser struct {
	videoDriver driver.Driver
	videoReader video.Reader
}

func (vrc videoReadCloser) Read() (img image.Image, release func(), err error) {
	return vrc.videoReader.Read()
}

func (vrc videoReadCloser) Close() error {
	return vrc.videoDriver.Close()
}

// A VideoReadReleaser automatically releases all images it reads from
// the underlying VideoReadCloser.
type VideoReadReleaser struct {
	VideoReadCloser
}

// Read replaces VideoReadCloser's Read with one that automatically
// releases the image it reads by cloning it.
func (vrr VideoReadReleaser) Read() (img image.Image, err error) {
	img, release, err := vrr.VideoReadCloser.Read()
	if err != nil {
		return nil, err
	}
	cloned := cloneImage(img)
	release()
	return cloned, nil
}

// to RGBA, may be lossy
func cloneImage(src image.Image) image.Image {
	bounds := src.Bounds()
	dst := image.NewRGBA(bounds)
	draw.Draw(dst, bounds, src, bounds.Min, draw.Src)
	return dst
}

// Next returns the next image read. This method satisfies APIs that use Next instead
// of Read with a given context.
func (vrr VideoReadReleaser) Next(ctx context.Context) (img image.Image, err error) {
	return vrr.Read()
}
