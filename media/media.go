package media

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pion/mediadevices/pkg/driver"
	"go.viam.com/utils"
)

// A Reader is anything that can read and recycle data.
type Reader[T any] interface {
	Read() (data T, release func(), err error)
}

type Stream[T any] interface {
	Next(ctx context.Context) (T, func(), error)
	Close()
}

// A ReadCloser is a T.Reader that requires it be closed.
type ReadCloser[T any, U any] interface {
	Reader[T]

	// Next returns the next media read. This method satisfies APIs that use Next instead
	// of Read with a given context. The release function should be called once the
	// media no longer will be utilized.
	Next(ctx context.Context) (T, func(), error)
	Stream(ctx context.Context) (Stream[T], error)
	Properties(ctx context.Context) (U, error)
	// Close cleans up any associated resources with the audio.Reader (e.g. a Driver).
	Close() error
}

type mediaReadCloser[T any, U any] struct {
	count                   int
	mu                      sync.Mutex
	driver                  driver.Driver
	reader                  Reader[T]
	props                   U
	cancelCtx               context.Context
	cancel                  func()
	activeBackgroundWorkers sync.WaitGroup
	current                 atomic.Value
	ready                   chan struct{}
	cond                    *sync.Cond
	triggerStartOnce        sync.Once
}

// newReadCloser instantiates a new media read closer and references the given
// driver.
func newReadCloser[T any, U any](d driver.Driver, r Reader[T], p U) ReadCloser[T, U] {
	driverRefs.mu.Lock()
	defer driverRefs.mu.Unlock()

	label := d.Info().Label
	if rcv, ok := driverRefs.refs[label]; ok {
		rcv.Ref()
	} else {
		driverRefs.refs[label] = utils.NewRefCountedValue(d)
		driverRefs.refs[label].Ref()
	}

	cancelCtx, cancel := context.WithCancel(context.Background())
	cond := sync.NewCond(&sync.RWMutex{})
	mrc := &mediaReadCloser[T, U]{
		driver:    d,
		reader:    r,
		props:     p,
		cancelCtx: cancelCtx,
		cancel:    cancel,
		ready:     make(chan struct{}, 1),
		cond:      cond,
	}
	mrc.start(cancelCtx)
	return mrc
}

func (mrc *mediaReadCloser[T, U]) start(ctx context.Context) {
	mrc.activeBackgroundWorkers.Add(1)
	utils.ManagedGo(func() {
		first := true
		for {
			select {
			case <-ctx.Done():
				return
			case <-mrc.ready:
			}

			now := time.Now()
			media, release, err := mrc.Read()
			fmt.Println("took", time.Since(now))

			// TODO(erd): maybe copy and no release since we are fanning out and dont know
			// who will/won't receive/close
			mrc.current.Store(&mediaReleasePair[T]{media, release, err})
			if first {
				first = false
				close(mrc.ready)
			}
			mrc.cond.Broadcast()
		}
	}, func() { defer mrc.activeBackgroundWorkers.Done(); mrc.cancel() })
}

func (mrc *mediaReadCloser[T, U]) Read() (T, func(), error) {
	return mrc.reader.Read()
}

func (mrc *mediaReadCloser[T, U]) Properties(_ context.Context) (U, error) {
	fmt.Printf("PROPS %#v\n", mrc.props)
	return mrc.props, nil
}

func (mrc *mediaReadCloser[T, U]) Next(ctx context.Context) (T, func(), error) {
	return mrc.Read()
}

func (mrc *mediaReadCloser[T, U]) triggerStart() {
	mrc.triggerStartOnce.Do(func() {
		mrc.ready <- struct{}{}
	})
}

type mediaReleasePair[T any] struct {
	media   T
	release func()
	err     error
}

type stream[T any, U any] struct {
	last      time.Time
	count     int
	mrc       *mediaReadCloser[T, U]
	cancelCtx context.Context
	cancel    func()
}

func (ms *stream[T, U]) Next(ctx context.Context) (T, func(), error) {
	var zero T
	if err := ms.cancelCtx.Err(); err != nil {
		return zero, nil, err
	}
	ms.mrc.triggerStart()
	select {
	case <-ms.cancelCtx.Done():
		return zero, nil, ms.cancelCtx.Err()
	case <-ctx.Done():
		return zero, nil, ctx.Err()
	case <-ms.mrc.ready:
	}
	ms.mrc.cond.L.Lock()
	if err := ms.cancelCtx.Err(); err != nil {
		return zero, nil, err
	}
	ms.mrc.cond.Wait()
	ms.mrc.cond.L.Unlock()

	now := time.Now()
	println("GOT", ms.count)
	fmt.Println("GOT", ms.count, now.Sub(ms.last))
	ms.last = now
	current := ms.mrc.current.Load().(*mediaReleasePair[T])
	return current.media, current.release, current.err
}

func (ms *stream[T, U]) Close() {
	ms.cancel()
}

func (mrc *mediaReadCloser[T, U]) Stream(ctx context.Context) (Stream[T], error) {
	mrc.count++
	cancelCtx, cancel := context.WithCancel(mrc.cancelCtx)
	stream := &stream[T, U]{
		count:     mrc.count,
		mrc:       mrc,
		cancelCtx: cancelCtx,
		cancel:    cancel,
	}

	return stream, nil
}

func (mrc *mediaReadCloser[T, U]) Close() error {
	mrc.mu.Lock()
	mrc.cancel()
	mrc.mu.Unlock()
	mrc.activeBackgroundWorkers.Wait()

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
