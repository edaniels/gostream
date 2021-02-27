package gostream

import (
	"errors"
	"fmt"
	"image"
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
	Video: func(constraint *mediadevices.MediaTrackConstraints) {},
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

func GetDisplayReader() (VideoReadCloser, error) {
	d, selectedMedia, err := GetDisplayDriver(defaultConstraints)
	if err != nil {
		return nil, err
	}
	return newVideoReaderFromDriver(d, selectedMedia)
}

func GetUserReader() (VideoReadCloser, error) {
	d, selectedMedia, err := GetUserDriver(defaultConstraints)
	if err != nil {
		return nil, err
	}
	return newVideoReaderFromDriver(d, selectedMedia)
}

func GetDisplayDriver(constraints mediadevices.MediaStreamConstraints) (driver.Driver, prop.Media, error) {
	var videoConstraints mediadevices.MediaTrackConstraints
	if constraints.Video != nil {
		constraints.Video(&videoConstraints)
	}
	return selectScreen(videoConstraints, constraints.Codec)
}

func GetUserDriver(constraints mediadevices.MediaStreamConstraints) (driver.Driver, prop.Media, error) {
	var videoConstraints mediadevices.MediaTrackConstraints
	if constraints.Video != nil {
		constraints.Video(&videoConstraints)
	}
	return selectVideo(videoConstraints, constraints.Codec)
}

func selectVideo(constraints mediadevices.MediaTrackConstraints, selector *mediadevices.CodecSelector) (driver.Driver, prop.Media, error) {
	typeFilter := driver.FilterVideoRecorder()
	notScreenFilter := driver.FilterNot(driver.FilterDeviceType(driver.Screen))
	filter := driver.FilterAnd(typeFilter, notScreenFilter)

	return selectBestDriver(filter, constraints)
}

func selectScreen(constraints mediadevices.MediaTrackConstraints, selector *mediadevices.CodecSelector) (driver.Driver, prop.Media, error) {
	typeFilter := driver.FilterVideoRecorder()
	screenFilter := driver.FilterDeviceType(driver.Screen)
	filter := driver.FilterAnd(typeFilter, screenFilter)

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
