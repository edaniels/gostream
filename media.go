package gostream

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/pion/mediadevices/pkg/driver"
	"go.uber.org/multierr"
	"go.viam.com/utils"
)

// A MediaReader is anything that can read and recycle data.
type MediaReader[T any] interface {
	Read() (data T, release func(), err error)
}

// A MediaStream streams media forever until closed.
type MediaStream[T any] interface {
	Next(ctx context.Context) (T, func(), error)
	Close(ctx context.Context) error
}

// A MediaSource can produce Streams of Ts.
type MediaSource[T any, U any] interface {
	// Next gets a single media from a stream.
	Next(ctx context.Context) (T, func(), error)
	Stream(ctx context.Context) (MediaStream[T], error)
	Properties(ctx context.Context) (U, error)
	// Close cleans up any associated resources with the Source (e.g. a Driver).
	Close(ctx context.Context) error
}

type mediaSource[T any, U any] struct {
	mu                      sync.Mutex
	driver                  driver.Driver
	reader                  MediaReader[T]
	props                   U
	cancelCtx               context.Context
	cancel                  func()
	activeBackgroundWorkers sync.WaitGroup
	current                 atomic.Value
	ready                   chan struct{}
	cond                    *sync.Cond
	triggerStartOnce        sync.Once
	copyFn                  func(src T) T
}

// newMediaSource instantiates a new media read closer and possibly references the given driver.
func newMediaSource[T any, U any](d driver.Driver, r MediaReader[T], p U, copyFn func(src T) T) MediaSource[T, U] {
	if d != nil {
		driverRefs.mu.Lock()
		defer driverRefs.mu.Unlock()

		label := d.Info().Label
		if rcv, ok := driverRefs.refs[label]; ok {
			rcv.Ref()
		} else {
			driverRefs.refs[label] = utils.NewRefCountedValue(d)
			driverRefs.refs[label].Ref()
		}
	}

	cancelCtx, cancel := context.WithCancel(context.Background())
	cond := sync.NewCond(&sync.RWMutex{})
	ms := &mediaSource[T, U]{
		driver:    d,
		reader:    r,
		props:     p,
		cancelCtx: cancelCtx,
		cancel:    cancel,
		ready:     make(chan struct{}, 1),
		cond:      cond,
		copyFn:    copyFn,
	}
	ms.start(cancelCtx)
	return ms
}

func (ms *mediaSource[T, U]) start(ctx context.Context) {
	ms.activeBackgroundWorkers.Add(1)
	utils.ManagedGo(func() {
		first := true
		for {
			select {
			case <-ctx.Done():
				return
			case <-ms.ready:
			}

			media, release, err := ms.reader.Read()
			ms.current.Store(&MediaReleasePairWithError[T]{media, release, err})
			if first {
				first = false
				close(ms.ready)
			}
			ms.cond.Broadcast()
		}
	}, func() { defer ms.activeBackgroundWorkers.Done(); ms.cancel() })
}

func (ms *mediaSource[T, U]) Next(ctx context.Context) (T, func(), error) {
	mediaStream, err := ms.Stream(ctx)
	var zero T
	if err != nil {
		return zero, nil, err
	}
	return mediaStream.Next(ctx)
}

func (ms *mediaSource[T, U]) Properties(_ context.Context) (U, error) {
	return ms.props, nil
}

func (ms *mediaSource[T, U]) triggerStart() {
	ms.triggerStartOnce.Do(func() {
		ms.ready <- struct{}{}
	})
}

// MediaReleasePairWithError contains the result of fetching media.
type MediaReleasePairWithError[T any] struct {
	Media   T
	Release func()
	Err     error
}

// NewMediaStreamForChannel returns a MediaStream backed by a channel.
func NewMediaStreamForChannel[T any](ctx context.Context) (context.Context, MediaStream[T], chan<- MediaReleasePairWithError[T]) {
	cancelCtx, cancel := context.WithCancel(ctx)
	ch := make(chan MediaReleasePairWithError[T])
	return cancelCtx, &mediaStreamFromChannel[T]{
		media:     ch,
		cancelCtx: cancelCtx,
		cancel:    cancel,
	}, ch
}

type mediaStreamFromChannel[T any] struct {
	media     chan MediaReleasePairWithError[T]
	cancelCtx context.Context
	cancel    func()
}

func (ms *mediaStreamFromChannel[T]) Next(ctx context.Context) (T, func(), error) {
	var zero T
	select {
	case <-ms.cancelCtx.Done():
		return zero, nil, ms.cancelCtx.Err()
	case <-ctx.Done():
		return zero, nil, ctx.Err()
	case pair := <-ms.media:
		return pair.Media, pair.Release, pair.Err
	}
}

func (ms *mediaStreamFromChannel[T]) Close(ctx context.Context) error {
	ms.cancel()
	return nil
}

type mediaStream[T any, U any] struct {
	ms        *mediaSource[T, U]
	cancelCtx context.Context
	cancel    func()
}

func (ms *mediaStream[T, U]) Next(ctx context.Context) (T, func(), error) {
	var zero T
	if err := ms.cancelCtx.Err(); err != nil {
		return zero, nil, err
	}
	ms.ms.triggerStart()
	select {
	case <-ms.cancelCtx.Done():
		return zero, nil, ms.cancelCtx.Err()
	case <-ctx.Done():
		return zero, nil, ctx.Err()
	case <-ms.ms.ready:
	}
	ms.ms.cond.L.Lock()
	if err := ms.cancelCtx.Err(); err != nil {
		return zero, nil, err
	}
	ms.ms.cond.Wait()
	ms.ms.cond.L.Unlock()

	current := ms.ms.current.Load().(*MediaReleasePairWithError[T])
	if current.Err != nil {
		return zero, nil, current.Err
	}
	defer current.Release()
	return ms.ms.copyFn(current.Media), func() {}, nil
}

func (ms *mediaStream[T, U]) Close(ctx context.Context) error {
	ms.cancel()
	return nil
}

func (ms *mediaSource[T, U]) Stream(ctx context.Context) (MediaStream[T], error) {
	cancelCtx, cancel := context.WithCancel(ms.cancelCtx)
	stream := &mediaStream[T, U]{
		ms:        ms,
		cancelCtx: cancelCtx,
		cancel:    cancel,
	}

	return stream, nil
}

func (ms *mediaSource[T, U]) Close(ctx context.Context) error {
	ms.mu.Lock()
	ms.cancel()
	ms.mu.Unlock()
	ms.activeBackgroundWorkers.Wait()
	err := utils.TryClose(ctx, ms.reader)

	if ms.driver == nil {
		return err
	}
	driverRefs.mu.Lock()
	defer driverRefs.mu.Unlock()

	label := ms.driver.Info().Label
	if rcv, ok := driverRefs.refs[label]; ok {
		if rcv.Deref() {
			delete(driverRefs.refs, label)
			return multierr.Combine(err, ms.driver.Close())
		}
	} else {
		return multierr.Combine(err, ms.driver.Close())
	}

	// Do not close if a driver is being referenced. Client will decide what to do if
	// they encounter this error.
	return multierr.Combine(err, &DriverInUseError{label})
}
