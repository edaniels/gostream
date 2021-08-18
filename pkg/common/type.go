package common

import (
	"image"
	"math"

	"github.com/nfnt/resize"
)

type CursorImage struct {
	Img    image.Image
	Width  int
	Height int
	Hotx   int
	Hoty   int
}

func (c CursorImage) Scale(factor float32) CursorImage {
	out := CursorImage{}
	out.Height = int(math.Round(float64(factor) * float64(c.Height)))
	out.Width = int(math.Round(float64(factor) * float64(c.Width)))
	out.Hotx = int(math.Round(float64(factor) * float64(c.Hotx)))
	out.Hoty = int(math.Round(float64(factor) * float64(c.Hoty)))
	out.Img = resize.Resize(uint(out.Width), uint(out.Height), c.Img, resize.Lanczos3)
	return out
}
