package gostream

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"os"
	"testing"

	"github.com/pion/mediadevices/pkg/prop"
	"go.viam.com/test"
)

type ImageSource struct {
	Images []image.Image
	idx    int
}

// Returns the next image or nil if there are no more images left. This should never error.
func (is *ImageSource) Read(_ context.Context) (image.Image, func(), error) {
	if is.idx >= len(is.Images) {
		return nil, func() {}, nil
	}
	img := is.Images[is.idx]
	is.idx++
	return img, func() {}, nil
}

// Nothing to do.
func (is *ImageSource) Close(_ context.Context) error {
	return nil
}

func PNGtoImage(t *testing.T, path string) image.Image {
	t.Helper()
	openBytes, err := os.ReadFile(path)
	test.That(t, err, test.ShouldBeNil)
	img, err := png.Decode(bytes.NewReader(openBytes))
	test.That(t, err, test.ShouldBeNil)
	return img
}

func TestReadMedia(t *testing.T) {
	colors := []image.Image{
		PNGtoImage(t, "data/red.png"),
		PNGtoImage(t, "data/blue.png"),
		PNGtoImage(t, "data/green.png"),
		PNGtoImage(t, "data/yellow.png"),
		PNGtoImage(t, "data/fuchsia.png"),
		PNGtoImage(t, "data/cyan.png"),
	}

	var imgSource ImageSource
	// Create a deep copy
	imgSource.Images = append(imgSource.Images, colors...)

	videoSrc := NewVideoSource(&imgSource, prop.Video{})
	// Test all images are returned in order.
	for _, expected := range colors {
		actual, _, err := ReadMedia(context.Background(), videoSrc)
		test.That(t, err, test.ShouldBeNil)
		test.That(t, actual, test.ShouldEqual, expected)
	}

	// Test image comparison can fail if two images are not the same
	imgSource.Images = []image.Image{PNGtoImage(t, "data/red.png")}
	videoSrc = NewVideoSource(&imgSource, prop.Video{})

	blue := PNGtoImage(t, "data/blue.png")
	red, _, err := ReadMedia(context.Background(), videoSrc)
	test.That(t, err, test.ShouldBeNil)
	test.That(t, red, test.ShouldNotEqual, blue)
}
