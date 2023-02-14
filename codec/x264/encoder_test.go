package x264

import (
	"bytes"
	"context"
	"github.com/edaniels/golog"
	"github.com/nfnt/resize"
	"go.viam.com/test"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"testing"
)

var width int
var height int
var logger golog.Logger
var imgCyan image.Image
var imgFuchsia image.Image
var w bool

const DefaultKeyFrameInterval = 30

func init() {
	width = 640
	height = 480
	imgCyan = resizeImg(pngToImage("../../data/cyan.png"), uint(width), uint(height))
	imgFuchsia = resizeImg(pngToImage("../../data/fuchsia.png"), uint(width), uint(height))
}

func pngToImage(path string) image.Image {
	openBytes, err := os.ReadFile(path)
	if err != nil {
		panic(err.Error())
	}
	img, err := png.Decode(bytes.NewReader(openBytes))
	if err != nil {
		panic(err.Error())
	}
	return img
}
func resizeImg(img image.Image, width uint, height uint) image.Image {
	newImage := resize.Resize(width, height, img, resize.Lanczos3)
	return newImage
}

func convertToYCbCr(b *testing.B, src image.Image) (image.Image, error) {
	bf := new(bytes.Buffer)
	err := jpeg.Encode(bf, src, nil)
	test.That(b, err, test.ShouldBeNil)
	dst, _, err := image.Decode(bf)
	test.That(b, err, test.ShouldBeNil)
	test.That(b, dst.ColorModel(), test.ShouldResemble, color.YCbCrModel)
	return dst, err

}

func BenchmarkEncodeRGBA(b *testing.B) {
	ctx := context.Background()
	encoder, err := NewEncoder(width, height, DefaultKeyFrameInterval, logger)
	test.That(b, err, test.ShouldBeNil)

	for i := 0; i < b.N; i++ {
		if w {
			_, err = encoder.Encode(ctx, imgCyan)
			test.That(b, err, test.ShouldBeNil)
		} else {
			_, err = encoder.Encode(ctx, imgFuchsia)
			test.That(b, err, test.ShouldBeNil)
		}
		w = !w
	}
}

func BenchmarkEncodeYCbCrs(b *testing.B) {
	imgFY, err := convertToYCbCr(b, imgFuchsia)
	test.That(b, err, test.ShouldBeNil)

	imgCY, err := convertToYCbCr(b, imgCyan)
	test.That(b, err, test.ShouldBeNil)

	encoder, err := NewEncoder(width, height, DefaultKeyFrameInterval, logger)
	test.That(b, err, test.ShouldBeNil)

	ctx := context.Background()
	for i := 0; i < b.N; i++ {
		if w {
			_, err = encoder.Encode(ctx, imgFY)
			test.That(b, err, test.ShouldBeNil)
		} else {
			_, err = encoder.Encode(ctx, imgCY)
			test.That(b, err, test.ShouldBeNil)
		}
		w = !w
	}
}
