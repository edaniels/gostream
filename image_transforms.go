package gostream

import (
	"context"
	"image"
	"image/color"
	"math"
	"sync"

	"github.com/disintegration/imaging"
	"github.com/edaniels/golog"
	"go.uber.org/multierr"
	"go.viam.com/utils"
)

// RotateImageSource rotates images by a set amount of degrees.
type RotateImageSource struct {
	Src         ImageSource
	RotateByDeg float64
}

// Next returns a rotated image by RotateByDeg degrees.
func (rms *RotateImageSource) Next(ctx context.Context) (image.Image, func(), error) {
	img, release, err := rms.Src.Next(ctx)
	if err != nil {
		return nil, nil, err
	}
	if release != nil {
		defer release()
	}

	return imaging.Rotate(img, rms.RotateByDeg, color.Black), func() {}, nil
}

// Close closes the underlying source.
func (rms *RotateImageSource) Close(ctx context.Context) error {
	return utils.TryClose(ctx, rms.Src)
}

// ResizeImageSource resizes images to the set dimensions.
type ResizeImageSource struct {
	Src           ImageSource
	Width, Height int
}

// Next returns a resized image to Width x Height dimensions.
func (ris ResizeImageSource) Next(ctx context.Context) (image.Image, func(), error) {
	img, release, err := ris.Src.Next(ctx)
	if err != nil {
		return nil, nil, err
	}
	if release != nil {
		defer release()
	}

	return imaging.Resize(img, ris.Width, ris.Height, imaging.NearestNeighbor), func() {}, nil
}

// Close closes the underlying source.
func (ris ResizeImageSource) Close(ctx context.Context) error {
	return utils.TryClose(ctx, ris.Src)
}

// An AutoTiler automatically tiles a series of images from sources. It rudimentarily
// swaps between veritcal and horizontal splits and makes all images the same size
// within a grid. This can produce aesthetically unappealing results but it gets
// the job done in a pinch where you don't want multiple streams. AutoTiler supports
// adding image sources over time (but not removing them yet).
type AutoTiler struct {
	mu        sync.Mutex
	sources   []ImageSource
	maxWidth  int
	maxHeight int
	vert      bool
	logger    golog.Logger
}

// NewAutoTiler returns an AutoTiler that adapts its image sources to the given width and height.
func NewAutoTiler(maxWidth, maxHeight int, sources ...ImageSource) *AutoTiler {
	return newAutoTiler(maxWidth, maxHeight, false, sources...)
}

// NewAutoTilerVertical returns an AutoTiler that adapts its image sources to the given width and height.
// This AutoTiler starts its splits vertically, instead of horizontally.
func NewAutoTilerVertical(maxWidth, maxHeight int, sources ...ImageSource) *AutoTiler {
	return newAutoTiler(maxWidth, maxHeight, true, sources...)
}

func newAutoTiler(maxWidth, maxHeight int, vert bool, sources ...ImageSource) *AutoTiler {
	return &AutoTiler{
		maxWidth:  maxWidth,
		maxHeight: maxHeight,
		sources:   sources,
		vert:      vert,
	}
}

// SetLogger sets an optional logger to use for debug information.
func (at *AutoTiler) SetLogger(logger golog.Logger) {
	at.mu.Lock()
	at.logger = logger
	at.mu.Unlock()
}

// AddSource adds another image source to the tiler. It will appear down and to
// the right of the main image.
func (at *AutoTiler) AddSource(src ImageSource) {
	at.mu.Lock()
	at.sources = append(at.sources, src)
	at.mu.Unlock()
}

// Next produces an image of every source tiled into one main image. If any of
// the image sources error, the image will not be included in the main image
// but it can certainly appear in subsequent ones. Images are fetched in
// parallel with no current constraint on parallelism.
func (at *AutoTiler) Next(ctx context.Context) (image.Image, func(), error) {
	at.mu.Lock()
	defer at.mu.Unlock()

	allImgs := make([]image.Image, len(at.sources))
	allReleases := make([]func(), len(at.sources))
	fs := make([]func() error, 0, len(at.sources))

	for i, src := range at.sources {
		iCopy := i
		srcCopy := src
		fs = append(fs, func() error {
			img, release, err := srcCopy.Next(ctx)
			if err != nil {
				return err
			}
			allImgs[iCopy] = img
			allReleases[iCopy] = release
			return nil
		})
	}
	if err := runParallel(fs); err != nil {
		if at.logger != nil {
			at.logger.Debugw("error grabbing frames", "error", err)
		}
	}
	for _, r := range allReleases {
		if r == nil {
			continue
		}
		defer r()
	}

	// We want to divide our space into alternating
	// splits against x and y. We can do this with some
	// lagging math where we find two factors of our number
	// that are greater than or equal to it. Rounding the square
	// root as x causes it to lag behind the ceil of the square
	// root as y.
	sqrt := math.Sqrt(float64(len(allImgs)))
	var xFill float64
	var yFill float64
	if at.vert {
		xFill = math.Ceil(sqrt)
		yFill = math.Round(sqrt)
	} else {
		xFill = math.Round(sqrt)
		yFill = math.Ceil(sqrt)
	}
	xStride := float64(at.maxWidth) / xFill
	yStride := float64(at.maxHeight) / yFill

	dst := imaging.New(at.maxWidth, at.maxHeight, color.NRGBA{0, 0, 0, 0})
	var imgNum int
	for x := float64(0); x < float64(at.maxWidth); x += xStride {
		for y := float64(0); imgNum < len(allImgs) && y < float64(at.maxHeight); y += yStride {
			if allImgs[imgNum] == nil {
				continue // blank
			}
			resized := imaging.Resize(allImgs[imgNum], int(xStride), int(yStride), imaging.NearestNeighbor)
			dst = imaging.Paste(dst, resized, image.Pt(int(x), int(y)))
			imgNum++
		}
	}
	return dst, func() {}, nil
}

// Close closes all underlying image sources.
func (at *AutoTiler) Close(ctx context.Context) error {
	var err error
	for _, src := range at.sources {
		err = multierr.Append(err, utils.TryClose(ctx, src))
	}
	return err
}
