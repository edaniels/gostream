package media

import (
	"context"

	"github.com/pion/mediadevices/pkg/driver"
	"go.viam.com/utils"
)

// A Reader is anything that can read and recycle data.
type Reader[T any] interface {
	Read() (data T, release func(), err error)
}

// A ReadCloser is a T.Reader that requires it be closed.
type ReadCloser[T any] interface {
	Reader[T]

	// Next returns the next media read. This method satisfies APIs that use Next instead
	// of Read with a given context. The release function should be called once the
	// media no longer will be utilized.
	Next(ctx context.Context) (T, func(), error)
	// Close cleans up any associated resources with the audio.Reader (e.g. a Driver).
	Close() error
}

type mediaReadCloser[T any] struct {
	driver driver.Driver
	reader Reader[T]
}

// newReadCloser instantiates a new media read closer and references the given
// driver.
func newReadCloser[T any](d driver.Driver, r Reader[T]) ReadCloser[T] {
	driverRefs.mu.Lock()
	defer driverRefs.mu.Unlock()

	label := d.Info().Label
	if rcv, ok := driverRefs.refs[label]; ok {
		rcv.Ref()
	} else {
		driverRefs.refs[label] = utils.NewRefCountedValue(d)
		driverRefs.refs[label].Ref()
	}

	return &mediaReadCloser[T]{d, r}
}

func (mrc mediaReadCloser[T]) Read() (T, func(), error) {
	return mrc.reader.Read()
}

func (mrc mediaReadCloser[T]) Next(ctx context.Context) (T, func(), error) {
	return mrc.Read()
}

func (mrc mediaReadCloser[T]) Close() error {
	driverRefs.mu.Lock()
	defer driverRefs.mu.Unlock()

	label := mrc.driver.Info().Label
	if rcv, ok := driverRefs.refs[label]; ok {
		if rcv.Deref() {
			delete(driverRefs.refs, label)
			return mrc.driver.Close()
		}
	} else {
		return mrc.driver.Close()
	}

	// Do not close if a driver is being referenced. Client will decide what to do if
	// they encounter this error.
	return &DriverInUseError{label}
}
