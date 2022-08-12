package media

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/pion/mediadevices/pkg/driver"
	"go.uber.org/multierr"
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

// A Source can produce Streams of Ts.
type Source[T any, U any] interface {
	// TODO(erd): REMOVE THIS but it breaks some image stuff
	Next(ctx context.Context) (T, func(), error)
	Stream(ctx context.Context) (Stream[T], error)
	Properties(ctx context.Context) (U, error)
	// Close cleans up any associated resources with the Source (e.g. a Driver).
	Close(ctx context.Context) error
}

type mediaSource[T any, U any] struct {
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
	copyFn                  func(src T) T
}

// newSource instantiates a new media read closer and possibly references the given driver.
func newSource[T any, U any](d driver.Driver, r Reader[T], p U, copyFn func(src T) T) Source[T, U] {
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
			ms.current.Store(&mediaReleasePair[T]{media, release, err})
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
	fmt.Printf("PROPS %#v\n", ms.props)
	return ms.props, nil
}

func (ms *mediaSource[T, U]) triggerStart() {
	ms.triggerStartOnce.Do(func() {
		ms.ready <- struct{}{}
	})
}

type mediaReleasePair[T any] struct {
	media   T
	release func()
	err     error
}

type stream[T any, U any] struct {
	ms        *mediaSource[T, U]
	cancelCtx context.Context
	cancel    func()
}

func (s *stream[T, U]) Next(ctx context.Context) (T, func(), error) {
	var zero T
	if err := s.cancelCtx.Err(); err != nil {
		return zero, nil, err
	}
	s.ms.triggerStart()
	select {
	case <-s.cancelCtx.Done():
		return zero, nil, s.cancelCtx.Err()
	case <-ctx.Done():
		return zero, nil, ctx.Err()
	case <-s.ms.ready:
	}
	s.ms.cond.L.Lock()
	if err := s.cancelCtx.Err(); err != nil {
		return zero, nil, err
	}
	s.ms.cond.Wait()
	s.ms.cond.L.Unlock()

	current := s.ms.current.Load().(*mediaReleasePair[T])
	if current.err != nil {
		return zero, nil, current.err
	}
	defer current.release()
	return s.ms.copyFn(current.media), func() {}, nil
}

func (s *stream[T, U]) Close() {
	s.cancel()
}

func (ms *mediaSource[T, U]) Stream(ctx context.Context) (Stream[T], error) {
	cancelCtx, cancel := context.WithCancel(ms.cancelCtx)
	stream := &stream[T, U]{
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
