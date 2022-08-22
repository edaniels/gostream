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
	Read(ctx context.Context) (data T, release func(), err error)
}

// A MediaReaderFunc is a helper to turn a function into a MediaReader.
type MediaReaderFunc[T any] func(ctx context.Context) (T, func(), error)

// Read calls the underlying function to get a media.
func (mrf MediaReaderFunc[T]) Read(ctx context.Context) (T, func(), error) {
	return mrf(ctx)
}

// A mediaReaderFuncNoCtx is a helper to turn a function into a MediaReader that cannot
// accept a context argument.
type mediaReaderFuncNoCtx[T any] func() (T, func(), error)

// Read calls the underlying function to get a media.
func (mrf mediaReaderFuncNoCtx[T]) Read(_ context.Context) (T, func(), error) {
	return mrf()
}

// A MediaStream streams media forever until closed.
type MediaStream[T any] interface {
	// Next returns the next media element in the sequence (best effort).
	Next(ctx context.Context) (T, func(), error)

	// Close signals this stream is no longer needed and releases associated
	// resources.
	Close(ctx context.Context) error
}

// A MediaSource can produce Streams of Ts.
type MediaSource[T any, U any] interface {
	// Read gets a single media from a stream. Using this has less of a guarantee
	// than Stream that the Nth media element follows the N-1th media element.
	Read(ctx context.Context) (T, func(), error)

	// Stream returns a stream that makes a best effort to return consecutive media element.
	Stream(ctx context.Context, errHandlers ...ErrorHandler) (MediaStream[T], error)

	// Properties returns information about the source.
	Properties(ctx context.Context) (U, error)

	// Close cleans up any associated resources with the Source (e.g. a Driver).
	Close(ctx context.Context) error
}

type mediaSource[T any, U any] struct {
	mu                      sync.Mutex
	driver                  driver.Driver
	reader                  MediaReader[T]
	readWrapper             MediaReader[T]
	props                   U
	cancelCtx               context.Context
	cancel                  func()
	activeBackgroundWorkers sync.WaitGroup
	current                 atomic.Value
	ready                   chan struct{}
	cond                    *sync.Cond
	startOnce               sync.Once
	copyFn                  func(src T) T
	errHandlers             map[*mediaStream[T, U]][]ErrorHandler
}

// ErrorHandler receives the error returned by a TSource.Next
// regardless of whether or not the error is nil (This allows
// for error handling logic based on consecutively retrieved errors).
type ErrorHandler func(ctx context.Context, mediaErr error)

// newMediaSource instantiates a new media read closer and possibly references the given driver.
func newMediaSource[T, U any](d driver.Driver, r MediaReader[T], p U, copyFn func(src T) T) MediaSource[T, U] {
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
		driver:      d,
		reader:      r,
		props:       p,
		cancelCtx:   cancelCtx,
		cancel:      cancel,
		ready:       make(chan struct{}),
		cond:        cond,
		copyFn:      copyFn,
		errHandlers: map[*mediaStream[T, U]][]ErrorHandler{},
	}
	ms.readWrapper = MediaReaderFunc[T](func(ctx context.Context) (T, func(), error) {
		media, release, err := ms.reader.Read(ctx)
		if err == nil {
			return media, release, nil
		}

		ms.mu.Lock()
		defer ms.mu.Unlock()
		for _, handlers := range ms.errHandlers {
			for _, handler := range handlers {
				handler(ctx, err)
			}
		}
		var zero T
		return zero, nil, err
	})
	return ms
}

func (ms *mediaSource[T, U]) start() {
	ms.startOnce.Do(func() {
		ms.activeBackgroundWorkers.Add(1)
		utils.ManagedGo(func() {
			first := true
			for {
				if ms.cancelCtx.Err() != nil {
					return
				}

				media, release, err := ms.readWrapper.Read(ms.cancelCtx)
				ms.current.Store(&MediaReleasePairWithError[T]{media, release, err})
				if first {
					first = false
					close(ms.ready)
				}
				ms.cond.Broadcast()
			}
		}, func() { defer ms.activeBackgroundWorkers.Done(); ms.cancel() })
	})
}

func (ms *mediaSource[T, U]) Read(ctx context.Context) (T, func(), error) {
	return ms.readWrapper.Read(ctx)
}

func (ms *mediaSource[T, U]) Properties(_ context.Context) (U, error) {
	return ms.props, nil
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
	ms.ms.start()
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
	ms.ms.mu.Lock()
	delete(ms.ms.errHandlers, ms)
	ms.ms.mu.Unlock()
	return nil
}

func (ms *mediaSource[T, U]) Stream(ctx context.Context, errHandlers ...ErrorHandler) (MediaStream[T], error) {
	cancelCtx, cancel := context.WithCancel(ms.cancelCtx)
	stream := &mediaStream[T, U]{
		ms:        ms,
		cancelCtx: cancelCtx,
		cancel:    cancel,
	}

	if len(errHandlers) != 0 {
		ms.mu.Lock()
		ms.errHandlers[stream] = errHandlers
		ms.mu.Unlock()
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
