package gostream

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"

	"github.com/pion/mediadevices/pkg/driver"
	"go.uber.org/multierr"
	"go.viam.com/utils"
)

type (
	// A MediaReader is anything that can read and recycle data. It is expected
	// that reader can handle multiple reads at the same time. This would ideally only
	// happen during streaming when a specific MIME type is requested. In the future,
	// we may be able to notice multiple MIME types and either do deferred encode/decode
	// or have the reader do it for us.
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
		// Stream returns a stream that makes a best effort to return consecutive media elements
		// that may have a MIME type hint dictated in the context via WithMIMETypeHint.
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
	driver        driver.Driver
	reader        MediaReader[T]
	props         U
	rootCancelCtx context.Context
	rootCancel    func()

	producerConsumers   map[string]*producerConsumer[T, U]
	producerConsumersMu sync.Mutex
}

type producerConsumer[T any, U any] struct {
	rootCancelCtx           context.Context
	cancelCtx               context.Context
	cancel                  func()
	mimeType                string
	activeBackgroundWorkers sync.WaitGroup
	readWrapper             MediaReader[T]
	current                 atomic.Value
	producerCond            *sync.Cond
	consumerCond            *sync.Cond
	condMu                  *sync.RWMutex
	interestedConsumers     int64
	errHandlers             map[*mediaStream[T, U]][]ErrorHandler
	listeners               int
	stateMu                 sync.Mutex
	listenersMu             sync.Mutex
	errHandlersMu           sync.Mutex
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
	ms := &mediaSource[T, U]{
		driver:            d,
		reader:            r,
		props:             p,
		rootCancelCtx:     cancelCtx,
		rootCancel:        cancel,
		producerConsumers: map[string]*producerConsumer[T, U]{},
	}
	return ms
}

func (pc *producerConsumer[T, U]) start() {
	pc.listenersMu.Lock()
	defer pc.listenersMu.Unlock()

	pc.listeners++

	if pc.listeners != 1 {
		return
	}

	pc.activeBackgroundWorkers.Add(1)

	utils.ManagedGo(func() {
		first := true
		for {
			if pc.cancelCtx.Err() != nil {
				return
			}

			pc.producerCond.L.Lock()
			requests := atomic.LoadInt64(&pc.interestedConsumers)
			if requests == 0 {
				pc.producerCond.Wait()
				pc.producerCond.L.Unlock()
				if err := pc.cancelCtx.Err(); err != nil {
					return
				}
			} else {
				pc.producerCond.L.Unlock()
			}

			func() {
				defer func() {
					pc.producerCond.L.Lock()
					atomic.AddInt64(&pc.interestedConsumers, -requests)
					pc.consumerCond.Broadcast()
					pc.producerCond.L.Unlock()
				}()

				var lastRelease func()
				if !first {
					lastRelease = pc.current.Load().(*MediaReleasePairWithError[T]).Release
				} else {
					first = false
				}
				media, release, err := pc.readWrapper.Read(pc.cancelCtx)
				pc.current.Store(&MediaReleasePairWithError[T]{media, release, err})
				if lastRelease != nil {
					lastRelease()
				}
			}()
		}
	}, func() { defer pc.activeBackgroundWorkers.Done(); pc.cancel() })
}

func (pc *producerConsumer[T, U]) stop() {
	pc.stateMu.Lock()
	defer pc.stateMu.Unlock()

	pc.cancel()

	pc.producerCond.L.Lock()
	pc.producerCond.Signal()
	pc.producerCond.L.Unlock()
	pc.consumerCond.L.Lock()
	pc.consumerCond.Broadcast()
	pc.consumerCond.L.Unlock()
	pc.activeBackgroundWorkers.Wait()

	// reset
	cancelCtx, cancel := context.WithCancel(WithMIMETypeHint(pc.rootCancelCtx, pc.mimeType))
	pc.cancelCtx = cancelCtx
	pc.cancel = cancel
}

func (pc *producerConsumer[T, U]) stopOne() {
	pc.listenersMu.Lock()
	defer pc.listenersMu.Unlock()
	pc.listeners--
	if pc.listeners == 0 {
		pc.stop()
	}
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
	prodCon   *producerConsumer[T, U]
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

	ms.prodCon.consumerCond.L.Lock()
	// Even though interestedConsumers is atomic, this is a critical section!
	// That's because if the producer sees zero interested consumers, it's going
	// to Wait but we only want it to do that once we are ready to signal it.
	// It's also a RLock because we have many consumers (readers) and one producer (writer).
	atomic.AddInt64(&ms.prodCon.interestedConsumers, 1)
	ms.prodCon.producerCond.Signal()

	select {
	case <-ms.cancelCtx.Done():
		ms.prodCon.consumerCond.L.Unlock()
		return zero, nil, ms.cancelCtx.Err()
	case <-ctx.Done():
		ms.prodCon.consumerCond.L.Unlock()
		return zero, nil, ctx.Err()
	default:
	}

	waitForNext := func() error {
		ms.prodCon.consumerCond.Wait()
		ms.prodCon.consumerCond.L.Unlock()
		if err := ms.cancelCtx.Err(); err != nil {
			return err
		}
		return nil
	}

	if err := waitForNext(); err != nil {
		return zero, nil, err
	}

	for ms.prodCon.current.Load() == nil {
		if err := waitForNext(); err != nil {
			return zero, nil, err
		}
	}

	current := ms.prodCon.current.Load().(*MediaReleasePairWithError[T])
	if current.Err != nil {
		return zero, nil, current.Err
	}
	return current.Media, func() {}, nil
}

func (ms *mediaStream[T, U]) Close(ctx context.Context) error {
	ms.cancel()
	ms.prodCon.errHandlersMu.Lock()
	delete(ms.prodCon.errHandlers, ms)
	ms.prodCon.errHandlersMu.Unlock()
	ms.prodCon.stopOne()
	return nil
}

func (ms *mediaSource[T, U]) Stream(ctx context.Context, errHandlers ...ErrorHandler) (MediaStream[T], error) {
	ms.producerConsumersMu.Lock()
	mimeType := MIMETypeHint(ctx, "")
	prodCon, ok := ms.producerConsumers[mimeType]
	if !ok {
		// TODO(erd): better to have no max like this and instead clean up over time.
		if len(ms.producerConsumers)+1 == 256 {
			return nil, errors.New("reached max producer consumers of 256")
		}
		cancelCtx, cancel := context.WithCancel(WithMIMETypeHint(ms.rootCancelCtx, mimeType))
		condMu := &sync.RWMutex{}
		producerCond := sync.NewCond(condMu)
		consumerCond := sync.NewCond(condMu.RLocker())

		prodCon = &producerConsumer[T, U]{
			rootCancelCtx: ms.rootCancelCtx,
			cancelCtx:     cancelCtx,
			cancel:        cancel,
			mimeType:      mimeType,
			producerCond:  producerCond,
			consumerCond:  consumerCond,
			condMu:        condMu,
			errHandlers:   map[*mediaStream[T, U]][]ErrorHandler{},
		}
		prodCon.readWrapper = MediaReaderFunc[T](func(ctx context.Context) (T, func(), error) {
			media, release, err := ms.reader.Read(ctx)
			if err == nil {
				return media, release, nil
			}

			prodCon.errHandlersMu.Lock()
			defer prodCon.errHandlersMu.Unlock()
			for _, handlers := range prodCon.errHandlers {
				for _, handler := range handlers {
					handler(ctx, err)
				}
			}
			var zero T
			return zero, nil, err
		})
		ms.producerConsumers[mimeType] = prodCon
	}
	ms.producerConsumersMu.Unlock()

	prodCon.stateMu.Lock()
	defer prodCon.stateMu.Unlock()

	cancelCtx, cancel := context.WithCancel(prodCon.cancelCtx)
	stream := &mediaStream[T, U]{
		ms:        ms,
		prodCon:   prodCon,
		cancelCtx: cancelCtx,
		cancel:    cancel,
	}

	if len(errHandlers) != 0 {
		prodCon.errHandlersMu.Lock()
		prodCon.errHandlers[stream] = errHandlers
		prodCon.errHandlersMu.Unlock()
	}
	prodCon.start()

	return stream, nil
}

func (ms *mediaSource[T, U]) Close(ctx context.Context) error {
	func() {
		ms.producerConsumersMu.Lock()
		defer ms.producerConsumersMu.Unlock()
		for _, prodCon := range ms.producerConsumers {
			prodCon.stop()
		}
	}()
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
