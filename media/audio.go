package media

import (
	"context"

	"github.com/pion/mediadevices/pkg/driver"
	"github.com/pion/mediadevices/pkg/io/audio"
	"github.com/pion/mediadevices/pkg/wave"
	"go.viam.com/utils"
)

// A AudioReadCloser is a audio.Reader that requires it be closed.
type AudioReadCloser interface {
	audio.Reader
	// Next returns the next image read. This method satisfies APIs that use Next instead
	// of Read with a given context. The release function should be called once the
	// image no longer will be utilized.
	Next(ctx context.Context) (wave.Audio, func(), error)
	// Close cleans up any associated resources with the audio.Reader (e.g. a Driver).
	Close() error
}

type audioReadCloser struct {
	audioDriver driver.Driver
	audioReader audio.Reader
}

// NewAudioReadCloser instantiates a new video read closer and references the given
// driver.
func NewAudioReadCloser(d driver.Driver, r audio.Reader) AudioReadCloser {
	driverRefs.mu.Lock()
	defer driverRefs.mu.Unlock()

	label := d.Info().Label
	if rcv, ok := driverRefs.refs[label]; ok {
		rcv.Ref()
	} else {
		driverRefs.refs[label] = utils.NewRefCountedValue(d)
		driverRefs.refs[label].Ref()
	}

	return &audioReadCloser{d, r}
}

func (arc audioReadCloser) Read() (wave.Audio, func(), error) {
	return arc.audioReader.Read()
}

func (arc audioReadCloser) Next(ctx context.Context) (wave.Audio, func(), error) {
	return arc.Read()
}

func (arc audioReadCloser) Close() error {
	driverRefs.mu.Lock()
	defer driverRefs.mu.Unlock()

	label := arc.audioDriver.Info().Label
	if rcv, ok := driverRefs.refs[label]; ok {
		if rcv.Deref() {
			delete(driverRefs.refs, label)
			return arc.audioDriver.Close()
		}
	} else {
		return arc.audioDriver.Close()
	}

	// Do not close if a driver is being referenced. Client will decide what to do if
	// they encounter this error.
	return &DriverInUseError{label}
}
