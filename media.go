package gostream

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/pion/mediadevices/pkg/driver"
	"go.uber.org/multierr"
	"go.viam.com/utils"
)

type (
	// A MediaReader is anything that can read and recycle data.
	MediaReader[T any] interface {
		Read(ctx context.Context) (data T, release func(), err error)
	}

	// A MediaReaderFunc is a helper to turn a function into a MediaReader.
	MediaReaderFunc[T any] func(ctx context.Context) (T, func(), error)

	// A MediaStream streams media forever until closed.
	MediaStream[T any] interface {
		// Next returns the next media element in the sequence (best effort).
		// Note: This element is mutable and shared globally; it MUST be copied
		// before it is mutated.
		Next(ctx context.Context) (T, func(), error)

		// Close signals this stream is no longer needed and releases associated
		// resources.
		Close(ctx context.Context) error
	}

	// A MediaSource can produce Streams of Ts.
	MediaSource[T any] interface {
		// Stream returns a stream that makes a best effort to return consecutive media element.
		Stream(ctx context.Context, errHandlers ...ErrorHandler) (MediaStream[T], error)

		// Close cleans up any associated resources with the Source (e.g. a Driver).
		Close(ctx context.Context) error
	}

	// MediaPropertyProvider providers information about a source.
	MediaPropertyProvider[U any] interface {
		MediaProperties(ctx context.Context) (U, error)
	}
)

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

// ReadMedia gets a single media from a source. Using this has less of a guarantee
// than MediaSource.Stream that the Nth media element follows the N-1th media element.
func ReadMedia[T any](ctx context.Context, source MediaSource[T]) (T, func(), error) {
	stream, err := source.Stream(ctx)
	var zero T
	if err != nil {
		return zero, nil, err
	}
	defer func() {
		utils.UncheckedError(stream.Close(ctx))
	}()
	return stream.Next(ctx)
}

type mediaSource[T any, U any] struct {
	driver                  driver.Driver
	reader                  MediaReader[T]
	readWrapper             MediaReader[T]
	props                   U
	cancelCtx               context.Context
	cancel                  func()
	activeBackgroundWorkers sync.WaitGroup
	current                 atomic.Value
	producerCond            *sync.Cond
	consumerCond            *sync.Cond
	condMu                  *sync.RWMutex
	interestedConsumers     atomic.Int64
	errHandlers             map[*mediaStream[T, U]][]ErrorHandler
	listeners               int

	stateMu       sync.Mutex
	listenersMu   sync.Mutex
	errHandlersMu sync.Mutex
}

// ErrorHandler receives the error returned by a TSource.Next
// regardless of whether or not the error is nil (This allows
// for error handling logic based on consecutively retrieved errors).
type ErrorHandler func(ctx context.Context, mediaErr error)

// newMediaSource instantiates a new media read closer and possibly references the given driver.
func newMediaSource[T, U any](d driver.Driver, r MediaReader[T], p U) MediaSource[T] {
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
	condMu := &sync.RWMutex{}
	producerCond := sync.NewCond(condMu)
	consumerCond := sync.NewCond(condMu.RLocker())
	ms := &mediaSource[T, U]{
		driver:       d,
		reader:       r,
		props:        p,
		cancelCtx:    cancelCtx,
		cancel:       cancel,
		producerCond: producerCond,
		consumerCond: consumerCond,
		condMu:       condMu,
		errHandlers:  map[*mediaStream[T, U]][]ErrorHandler{},
	}
	ms.readWrapper = MediaReaderFunc[T](func(ctx context.Context) (T, func(), error) {
		media, release, err := ms.reader.Read(ctx)
		if err == nil {
			return media, release, nil
		}

		ms.errHandlersMu.Lock()
		defer ms.errHandlersMu.Unlock()
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
	ms.listenersMu.Lock()
	defer ms.listenersMu.Unlock()
	ms.listeners++
	listeners := ms.listeners

	if listeners != 1 {
		return
	}

	ms.activeBackgroundWorkers.Add(1)

	utils.ManagedGo(func() {
		first := true
		for {
			if ms.cancelCtx.Err() != nil {
				return
			}

			ms.producerCond.L.Lock()
			requests := ms.interestedConsumers.Load()
			if requests == 0 {
				ms.producerCond.Wait()
				ms.producerCond.L.Unlock()
				if err := ms.cancelCtx.Err(); err != nil {
					return
				}
			} else {
				ms.producerCond.L.Unlock()
			}

			func() {
				defer func() {
					ms.producerCond.L.Lock()
					ms.interestedConsumers.Add(-requests)
					ms.consumerCond.Broadcast()
					ms.producerCond.L.Unlock()
				}()

				var lastRelease func()
				if !first {
					lastRelease = ms.current.Load().(*MediaReleasePairWithError[T]).Release
				} else {
					first = false
				}
				media, release, err := ms.readWrapper.Read(ms.cancelCtx)
				ms.current.Store(&MediaReleasePairWithError[T]{media, release, err})
				if lastRelease != nil {
					lastRelease()
				}
			}()
		}
	}, func() { defer ms.activeBackgroundWorkers.Done(); ms.cancel() })
}

func (ms *mediaSource[T, U]) stop() {
	ms.stateMu.Lock()
	defer ms.stateMu.Unlock()
	ms.cancel()

	ms.producerCond.L.Lock()
	ms.producerCond.Signal()
	ms.producerCond.L.Unlock()
	ms.consumerCond.L.Lock()
	ms.consumerCond.Broadcast()
	ms.consumerCond.L.Unlock()
	ms.activeBackgroundWorkers.Wait()

	// reset
	cancelCtx, cancel := context.WithCancel(context.Background())
	ms.cancelCtx = cancelCtx
	ms.cancel = cancel
}

func (ms *mediaSource[T, U]) stopOne() {
	ms.listenersMu.Lock()
	defer ms.listenersMu.Unlock()
	ms.listeners--
	listeners := ms.listeners
	if listeners == 0 {
		ms.stop()
	}
}

func (ms *mediaSource[T, U]) Read(ctx context.Context) (T, func(), error) {
	return ms.readWrapper.Read(ctx)
}

func (ms *mediaSource[T, U]) MediaProperties(_ context.Context) (U, error) {
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
	mu        sync.Mutex
	ms        *mediaSource[T, U]
	cancelCtx context.Context
	cancel    func()
}

func (ms *mediaStream[T, U]) Next(ctx context.Context) (T, func(), error) {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	// lock keeps us sequential and prevents misuse

	var zero T
	if err := ms.cancelCtx.Err(); err != nil {
		return zero, nil, err
	}

	ms.ms.consumerCond.L.Lock()
	// Even though interestedConsumers is atomic, this is a critical section!
	// That's because if the producer sees zero interested consumers, it's going
	// to Wait but we only want it to do that once we are ready to signal it.
	// It's also a RLock because we have many consumers (readers) and one producer (writer).
	ms.ms.interestedConsumers.Add(1)
	ms.ms.producerCond.Signal()

	select {
	case <-ms.cancelCtx.Done():
		ms.ms.consumerCond.L.Unlock()
		return zero, nil, ms.cancelCtx.Err()
	case <-ctx.Done():
		ms.ms.consumerCond.L.Unlock()
		return zero, nil, ctx.Err()
	default:
	}

	ms.ms.consumerCond.Wait()
	ms.ms.consumerCond.L.Unlock()
	if err := ms.cancelCtx.Err(); err != nil {
		return zero, nil, err
	}

	current := ms.ms.current.Load().(*MediaReleasePairWithError[T])
	if current.Err != nil {
		return zero, nil, current.Err
	}
	return current.Media, func() {}, nil
}

func (ms *mediaStream[T, U]) Close(ctx context.Context) error {
	ms.cancel()
	ms.ms.errHandlersMu.Lock()
	delete(ms.ms.errHandlers, ms)
	ms.ms.errHandlersMu.Unlock()
	ms.ms.stopOne()
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
		ms.errHandlersMu.Lock()
		ms.errHandlers[stream] = errHandlers
		ms.errHandlersMu.Unlock()
	}
	ms.start()

	return stream, nil
}

func (ms *mediaSource[T, U]) Close(ctx context.Context) error {
	ms.stop()
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
