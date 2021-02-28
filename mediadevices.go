package gostream

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/draw"
	"math"

	"github.com/pion/mediadevices"
	"github.com/pion/mediadevices/pkg/driver"
	"github.com/pion/mediadevices/pkg/io/video"
	"github.com/pion/mediadevices/pkg/prop"

	// register
	_ "github.com/pion/mediadevices/pkg/driver/camera"
	_ "github.com/pion/mediadevices/pkg/driver/screen"
)

// below adapted from github.com/pion/mediadevices

var errNotFound = fmt.Errorf("failed to find the best driver that fits the constraints")

var defaultConstraints = mediadevices.MediaStreamConstraints{
	Video: func(constraint *mediadevices.MediaTrackConstraints) {
		constraint.FrameRate = prop.Float(60)
	},
}

type VideoReadCloser interface {
	video.Reader
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

type VideoReadReleaser struct {
	VideoReadCloser
}

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

func (vrr VideoReadReleaser) Next(ctx context.Context) (img image.Image, err error) {
	return vrr.Read()
}

func newVideoReaderFromDriver(videoDriver driver.Driver, mediaProp prop.Media) (VideoReadCloser, error) {
	recorder, ok := videoDriver.(driver.VideoRecorder)
	if !ok {
		return nil, errors.New("driver not a driver.VideoRecorder")
	}
	if err := videoDriver.Open(); err != nil {
		return nil, err
	}
	reader, err := recorder.VideoRecord(mediaProp)
	if err != nil {
		return nil, err
	}
	return &videoReadCloser{videoDriver, reader}, nil
}

func GetNamedDisplayReader(name string) (VideoReadCloser, error) {
	d, selectedMedia, err := GetDisplayDriver(defaultConstraints, name)
	if err != nil {
		return nil, err
	}
	return newVideoReaderFromDriver(d, selectedMedia)
}

func GetNamedUserReader(name string) (VideoReadCloser, error) {
	d, selectedMedia, err := GetUserDriver(defaultConstraints, name)
	if err != nil {
		return nil, err
	}
	return newVideoReaderFromDriver(d, selectedMedia)
}

func GetDisplayReader() (VideoReadCloser, error) {
	d, selectedMedia, err := GetDisplayDriver(defaultConstraints, "")
	if err != nil {
		return nil, err
	}
	return newVideoReaderFromDriver(d, selectedMedia)
}

func GetUserReader() (VideoReadCloser, error) {
	d, selectedMedia, err := GetUserDriver(defaultConstraints, "")
	if err != nil {
		return nil, err
	}
	return newVideoReaderFromDriver(d, selectedMedia)
}

func GetDisplayDriver(constraints mediadevices.MediaStreamConstraints, label string) (driver.Driver, prop.Media, error) {
	var videoConstraints mediadevices.MediaTrackConstraints
	if constraints.Video != nil {
		constraints.Video(&videoConstraints)
	}
	return selectScreen(videoConstraints, label)
}

func GetUserDriver(constraints mediadevices.MediaStreamConstraints, label string) (driver.Driver, prop.Media, error) {
	var videoConstraints mediadevices.MediaTrackConstraints
	if constraints.Video != nil {
		constraints.Video(&videoConstraints)
	}
	return selectVideo(videoConstraints, label)
}

func selectVideo(constraints mediadevices.MediaTrackConstraints, label string) (driver.Driver, prop.Media, error) {
	typeFilter := driver.FilterVideoRecorder()
	notScreenFilter := driver.FilterNot(driver.FilterDeviceType(driver.Screen))
	filter := driver.FilterAnd(typeFilter, notScreenFilter)
	if label != "" {
		filter = driver.FilterAnd(filter, driver.FilterFn(func(d driver.Driver) bool {
			return d.Info().Label == label
		}))
	}

	return selectBestDriver(filter, constraints)
}

func selectScreen(constraints mediadevices.MediaTrackConstraints, label string) (driver.Driver, prop.Media, error) {
	typeFilter := driver.FilterVideoRecorder()
	screenFilter := driver.FilterDeviceType(driver.Screen)
	filter := driver.FilterAnd(typeFilter, screenFilter)
	if label != "" {
		filter = driver.FilterAnd(filter, driver.FilterFn(func(d driver.Driver) bool {
			return d.Info().Label == label
		}))
	}

	return selectBestDriver(filter, constraints)
}

// select implements SelectSettings algorithm.
// Reference: https://w3c.github.io/mediacapture-main/#dfn-selectsettings
func selectBestDriver(filter driver.FilterFn, constraints mediadevices.MediaTrackConstraints) (driver.Driver, prop.Media, error) {
	var bestDriver driver.Driver
	var bestProp prop.Media
	minFitnessDist := math.Inf(1)

	driverProperties := queryDriverProperties(filter)
	for d, props := range driverProperties {
		priority := float64(d.Info().Priority)
		for _, p := range props {
			fitnessDist, ok := constraints.MediaConstraints.FitnessDistance(p)
			if !ok {
				continue
			}
			fitnessDist -= priority
			if fitnessDist < minFitnessDist {
				minFitnessDist = fitnessDist
				bestDriver = d
				bestProp = p
			}
		}
	}

	if bestDriver == nil {
		return nil, prop.Media{}, errNotFound
	}

	selectedMedia := prop.Media{}
	selectedMedia.MergeConstraints(constraints.MediaConstraints)
	selectedMedia.Merge(bestProp)
	return bestDriver, selectedMedia, nil
}

func queryDriverProperties(filter driver.FilterFn) map[driver.Driver][]prop.Media {
	var needToClose []driver.Driver
	drivers := driver.GetManager().Query(filter)
	m := make(map[driver.Driver][]prop.Media)

	for _, d := range drivers {
		if d.Status() == driver.StateClosed {
			err := d.Open()
			if err != nil {
				// Skip this driver if we failed to open because we can't get the properties
				continue
			}
			needToClose = append(needToClose, d)
		}

		m[d] = d.Properties()
	}

	for _, d := range needToClose {
		// Since it was closed, we should close it to avoid a leak
		d.Close()
	}

	return m
}
