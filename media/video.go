package media

import (
	"context"
	"image"
	"sync"

	"github.com/edaniels/gostream"
	"github.com/pion/mediadevices/pkg/driver"
	"github.com/pion/mediadevices/pkg/io/video"
	"go.viam.com/utils"
)

// driverRefManager is a global map of drivers and how often they are referenced by video
// readers
type driverRefManager struct {
	refs map[string]utils.RefCountedValue
	mu   sync.Mutex
}

var driverRefs = driverRefManager{refs: map[string]utils.RefCountedValue{}}

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

// NewVideoReadCloser instantiates a new video read closer and references the given
// driver.
func NewVideoReadCloser(d driver.Driver, r video.Reader) *videoReadCloser {
	driverRefs.mu.Lock()
	defer driverRefs.mu.Unlock()

	label := d.Info().Label
	if rcv, ok := driverRefs.refs[label]; ok {
		// TODO: get the existing driver?
		rcv.Ref()
	} else {
		driverRefs.refs[label] = utils.NewRefCountedValue(d)
		// TODO: get the existing driver?
		driverRefs.refs[label].Ref()
	}

	return &videoReadCloser{d, r}
}

func (vrc videoReadCloser) Read() (image.Image, func(), error) {
	return vrc.videoReader.Read()
}

func (vrc videoReadCloser) Next(ctx context.Context) (image.Image, func(), error) {
	return vrc.Read()
}

func (vrc videoReadCloser) Close() error {
	driverRefs.mu.Lock()
	defer driverRefs.mu.Unlock()

	label := vrc.videoDriver.Info().Label
	if rcv, ok := driverRefs.refs[label]; ok {
		if rcv.Deref() {
			// TODO: get the driver from the refs map?
			return vrc.videoDriver.Close()
		}
	} else {
		// No known references, close the driver
		return vrc.videoDriver.Close()
	}

	// Do not close if a driver is being referenced.
	// TODO: should we throw an error here and let clients decide whether to proceed
	// silently or not?
	gostream.Logger.Warnw("driver is still in use, will not close", "driver", label)
	return nil
}
