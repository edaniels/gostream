// Package media contains media device API helpers.
package media

import (
	"errors"
	"math"
	"regexp"
	"strings"

	"github.com/pion/mediadevices"
	"github.com/pion/mediadevices/pkg/driver"
	"github.com/pion/mediadevices/pkg/driver/camera"
	"github.com/pion/mediadevices/pkg/frame"
	"github.com/pion/mediadevices/pkg/prop"

	"github.com/edaniels/gostream"
)

// below adapted from github.com/pion/mediadevices

// ErrNotFound happens when there is no driver found in a query.
var ErrNotFound = errors.New("failed to find the best driver that fits the constraints")

// DefaultConstraints are suitable for finding any available device.
var DefaultConstraints = mediadevices.MediaStreamConstraints{
	Video: func(constraint *mediadevices.MediaTrackConstraints) {
		constraint.Width = prop.IntRanged{640, 4096, 1920}
		constraint.Height = prop.IntRanged{400, 2160, 1080}
		constraint.FrameRate = prop.FloatRanged{0, 200, 60}
		constraint.FrameFormat = prop.FrameFormatOneOf{
			frame.FormatI420,
			frame.FormatI444,
			frame.FormatYUY2,
			frame.FormatUYVY,
			frame.FormatRGBA,
			frame.FormatMJPEG,
			frame.FormatNV12,
			frame.FormatZ16,
			frame.FormatNV21, // gives blue tinted image?
		}
	},
}

// GetNamedScreenReader attempts to find a screen device by the given name.
func GetNamedScreenReader(name string, constraints mediadevices.MediaStreamConstraints) (VideoReadCloser, error) {
	d, selectedMedia, err := getScreenDriver(constraints, &name)
	if err != nil {
		return nil, err
	}
	return newVideoReaderFromDriver(d, selectedMedia)
}

// GetPatternedScreenReader attempts to find a screen device by the given label pattern.
func GetPatternedScreenReader(labelPattern *regexp.Regexp, constraints mediadevices.MediaStreamConstraints) (VideoReadCloser, error) {
	d, selectedMedia, err := getScreenDriverPattern(constraints, labelPattern)
	if err != nil {
		return nil, err
	}
	return newVideoReaderFromDriver(d, selectedMedia)
}

// GetNamedVideoReader attempts to find a video device (not a screen) by the given name.
func GetNamedVideoReader(name string, constraints mediadevices.MediaStreamConstraints) (VideoReadCloser, error) {
	d, selectedMedia, err := getUserDriver(constraints, &name)
	if err != nil {
		return nil, err
	}
	return newVideoReaderFromDriver(d, selectedMedia)
}

// GetPatternedVideoReader attempts to find a video device (not a screen) by the given label pattern.
func GetPatternedVideoReader(labelPattern *regexp.Regexp, constraints mediadevices.MediaStreamConstraints) (VideoReadCloser, error) {
	d, selectedMedia, err := getUserDriverPattern(constraints, labelPattern)
	if err != nil {
		return nil, err
	}
	return newVideoReaderFromDriver(d, selectedMedia)
}

// GetAnyScreenReader attempts to find any suitable screen device.
func GetAnyScreenReader(constraints mediadevices.MediaStreamConstraints) (VideoReadCloser, error) {
	d, selectedMedia, err := getScreenDriver(constraints, nil)
	if err != nil {
		return nil, err
	}
	return newVideoReaderFromDriver(d, selectedMedia)
}

// GetAnyVideoReader attempts to find any suitable video device (not a screen).
func GetAnyVideoReader(constraints mediadevices.MediaStreamConstraints) (VideoReadCloser, error) {
	d, selectedMedia, err := getUserDriver(constraints, nil)
	if err != nil {
		return nil, err
	}
	return newVideoReaderFromDriver(d, selectedMedia)
}

// DeviceInfo describes a driver.
type DeviceInfo struct {
	ID         string
	Labels     []string
	Properties []prop.Media
	Priority   driver.Priority
	Error      error
}

// QueryVideoDevices lists all known video devices (not a screen).
func QueryVideoDevices() []DeviceInfo {
	return getDriverInfo(driver.GetManager().Query(getVideoFilterBase()), true)
}

// QueryScreenDevices lists all known screen devices.
func QueryScreenDevices() []DeviceInfo {
	return getDriverInfo(driver.GetManager().Query(getScreenFilterBase()), true)
}

func getDriverInfo(drivers []driver.Driver, useSep bool) []DeviceInfo {
	infos := make([]DeviceInfo, len(drivers))
	for i, d := range drivers {
		if d.Status() == driver.StateClosed {
			if err := d.Open(); err != nil {
				infos[i].Error = err
			} else {
				defer func() {
					infos[i].Error = d.Close()
				}()
			}
		}
		infos[i].ID = d.ID()
		infos[i].Labels = getDriverLabels(d, useSep)
		infos[i].Properties = d.Properties()
		infos[i].Priority = d.Info().Priority
	}
	return infos
}

// QueryScreenDevicesLabels lists all known screen devices.
func QueryScreenDevicesLabels() []string {
	return getDriversLabels(driver.GetManager().Query(getScreenFilterBase()), false)
}

// QueryVideoDeviceLabels lists all known video devices (not a screen).
func QueryVideoDeviceLabels() []string {
	return getDriversLabels(driver.GetManager().Query(getVideoFilterBase()), true)
}

func getDriversLabels(drivers []driver.Driver, useSep bool) []string {
	var labels []string
	for _, d := range drivers {
		labels = append(labels, getDriverLabels(d, useSep)...)
	}
	return labels
}

func getDriverLabels(d driver.Driver, useSep bool) []string {
	if !useSep {
		return []string{d.Info().Label}
	}
	return strings.Split(d.Info().Label, camera.LabelSeparator)
}

func getScreenDriver(constraints mediadevices.MediaStreamConstraints, label *string) (driver.Driver, prop.Media, error) {
	var videoConstraints mediadevices.MediaTrackConstraints
	if constraints.Video != nil {
		constraints.Video(&videoConstraints)
	}
	return selectScreen(videoConstraints, label)
}

func getScreenDriverPattern(
	constraints mediadevices.MediaStreamConstraints,
	labelPattern *regexp.Regexp,
) (driver.Driver, prop.Media, error) {
	var videoConstraints mediadevices.MediaTrackConstraints
	if constraints.Video != nil {
		constraints.Video(&videoConstraints)
	}
	return selectScreenPattern(videoConstraints, labelPattern)
}

func getUserDriver(constraints mediadevices.MediaStreamConstraints, label *string) (driver.Driver, prop.Media, error) {
	var videoConstraints mediadevices.MediaTrackConstraints
	if constraints.Video != nil {
		constraints.Video(&videoConstraints)
	}
	return selectVideo(videoConstraints, label)
}

func getUserDriverPattern(constraints mediadevices.MediaStreamConstraints, labelPattern *regexp.Regexp) (driver.Driver, prop.Media, error) {
	var videoConstraints mediadevices.MediaTrackConstraints
	if constraints.Video != nil {
		constraints.Video(&videoConstraints)
	}
	return selectVideoPattern(videoConstraints, labelPattern)
}

func newVideoReaderFromDriver(videoDriver driver.Driver, mediaProp prop.Media) (VideoReadCloser, error) {
	recorder, ok := videoDriver.(driver.VideoRecorder)
	if !ok {
		return nil, errors.New("driver not a driver.VideoRecorder")
	}

	if driverStatus := videoDriver.Status(); driverStatus != driver.StateClosed {
		gostream.Logger.Warnw("video driver is not closed, attempting to close and reopen", "status", driverStatus)
		if err := videoDriver.Close(); err != nil {
			gostream.Logger.Errorw("error closing driver", "error", err)
		}
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

func labelFilter(target string, useSep bool) driver.FilterFn {
	return driver.FilterFn(func(d driver.Driver) bool {
		if !useSep {
			return d.Info().Label == target
		}
		labels := strings.Split(d.Info().Label, camera.LabelSeparator)
		for _, label := range labels {
			if label == target {
				return true
			}
		}
		return false
	})
}

func labelFilterPattern(labelPattern *regexp.Regexp, useSep bool) driver.FilterFn {
	return driver.FilterFn(func(d driver.Driver) bool {
		if !useSep {
			return labelPattern.MatchString(d.Info().Label)
		}
		labels := strings.Split(d.Info().Label, camera.LabelSeparator)
		for _, label := range labels {
			if labelPattern.MatchString(label) {
				return true
			}
		}
		return false
	})
}

func selectVideo(constraints mediadevices.MediaTrackConstraints, label *string) (driver.Driver, prop.Media, error) {
	return selectBestDriver(getVideoFilter(label), constraints)
}

func selectVideoPattern(constraints mediadevices.MediaTrackConstraints, labelPattern *regexp.Regexp) (driver.Driver, prop.Media, error) {
	return selectBestDriver(getVideoFilterPattern(labelPattern), constraints)
}

func selectScreen(constraints mediadevices.MediaTrackConstraints, label *string) (driver.Driver, prop.Media, error) {
	return selectBestDriver(getScreenFilter(label), constraints)
}

func selectScreenPattern(constraints mediadevices.MediaTrackConstraints, labelPattern *regexp.Regexp) (driver.Driver, prop.Media, error) {
	return selectBestDriver(getScreenFilterPattern(labelPattern), constraints)
}

func getVideoFilterBase() driver.FilterFn {
	typeFilter := driver.FilterVideoRecorder()
	notScreenFilter := driver.FilterNot(driver.FilterDeviceType(driver.Screen))
	return driver.FilterAnd(typeFilter, notScreenFilter)
}

func getVideoFilter(label *string) driver.FilterFn {
	filter := getVideoFilterBase()
	if label != nil {
		filter = driver.FilterAnd(filter, labelFilter(*label, true))
	}
	return filter
}

func getVideoFilterPattern(labelPattern *regexp.Regexp) driver.FilterFn {
	filter := getVideoFilterBase()
	filter = driver.FilterAnd(filter, labelFilterPattern(labelPattern, true))
	return filter
}

func getScreenFilterBase() driver.FilterFn {
	typeFilter := driver.FilterVideoRecorder()
	screenFilter := driver.FilterDeviceType(driver.Screen)
	return driver.FilterAnd(typeFilter, screenFilter)
}

func getScreenFilter(label *string) driver.FilterFn {
	filter := getScreenFilterBase()
	if label != nil {
		filter = driver.FilterAnd(filter, labelFilter(*label, true))
	}
	return filter
}

func getScreenFilterPattern(labelPattern *regexp.Regexp) driver.FilterFn {
	filter := getScreenFilterBase()
	filter = driver.FilterAnd(filter, labelFilterPattern(labelPattern, true))
	return filter
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
		return nil, prop.Media{}, ErrNotFound
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
		if err := d.Close(); err != nil {
			gostream.Logger.Errorw("error closing driver", "error", err)
		}
	}

	return m
}
