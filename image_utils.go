package gostream

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"math"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/disintegration/imaging"
	"github.com/edaniels/golog"
	"go.uber.org/multierr"
)

type RotateImageSource struct {
	Original ImageSource
}

func (rms *RotateImageSource) Next(ctx context.Context) (image.Image, error) {
	img, err := rms.Original.Next(ctx)
	if err != nil {
		return nil, err
	}

	return imaging.Rotate(img, 180, color.Black), nil
}

func (rms *RotateImageSource) Close() {
	rms.Original.Close()
}

type ResizeImageSource struct {
	ImageSource
	X, Y int
}

func (ris ResizeImageSource) Next(ctx context.Context) (image.Image, error) {
	img, err := ris.ImageSource.Next(ctx)
	if err != nil {
		return nil, err
	}

	return imaging.Resize(img, ris.X, ris.Y, imaging.NearestNeighbor), nil
}

func (ris ResizeImageSource) Close() error {
	return ris.ImageSource.Close()
}

type AutoTiler struct {
	mu        sync.Mutex
	sources   []ImageSource
	maxWidth  int
	maxHeight int
	vert      bool
	logger    golog.Logger
}

func NewAutoTiler(maxWidth, maxHeight int, sources ...ImageSource) *AutoTiler {
	return &AutoTiler{
		maxWidth:  maxWidth,
		maxHeight: maxHeight,
		sources:   sources,
	}
}

func NewAutoTilerVertical(maxWidth, maxHeight int, sources ...ImageSource) *AutoTiler {
	return &AutoTiler{
		maxWidth:  maxWidth,
		maxHeight: maxHeight,
		sources:   sources,
		vert:      true,
	}
}

func (at *AutoTiler) SetLogger(logger golog.Logger) {
	at.mu.Lock()
	at.logger = logger
	at.mu.Unlock()
}

func (at *AutoTiler) AddSource(src ImageSource) {
	at.mu.Lock()
	at.sources = append(at.sources, src)
	at.mu.Unlock()
}

func (at *AutoTiler) Next(ctx context.Context) (image.Image, error) {
	at.mu.Lock()
	defer at.mu.Unlock()

	allImgs := make([]image.Image, 0, len(at.sources))
	fs := make([]func() error, 0, len(at.sources))

	for _, src := range at.sources {
		srcCopy := src
		fs = append(fs, func() error {
			img, err := srcCopy.Next(ctx)
			if err != nil {
				return err
			}
			allImgs = append(allImgs, img)
			return nil
		})
	}
	if err := RunParallel(fs); err != nil {
		if at.logger != nil {
			at.logger.Debugw("error grabbing frames", "error", err)
		}
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
			resized := imaging.Resize(allImgs[imgNum], int(xStride), int(yStride), imaging.NearestNeighbor)
			dst = imaging.Paste(dst, resized, image.Pt(int(x), int(y)))
			imgNum++
		}
	}
	return dst, nil
}

func (at *AutoTiler) Close() error {
	var err error
	for _, src := range at.sources {
		err = multierr.Append(err, src.Close())
	}
	return err
}

func RunParallel(fs []func() error) error {
	var wg sync.WaitGroup
	wg.Add(len(fs))
	errs := make([]error, len(fs))
	var numErrs int32
	for i, f := range fs {
		iCopy := i
		fCopy := f
		go func() {
			defer wg.Done()
			err := fCopy()
			if err != nil {
				errs[iCopy] = err
				atomic.AddInt32(&numErrs, 1)
			}
		}()
	}
	wg.Wait()

	if numErrs == 0 {
		return nil
	}
	var allErrs []interface{}
	for _, err := range errs {
		if err == nil {
			continue
		}
		allErrs = append(allErrs, err)
	}
	return fmt.Errorf("encountered errors:"+strings.Repeat(" %w", len(allErrs)), allErrs...)
}
