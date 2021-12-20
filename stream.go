package gostream

import (
	"context"
	"errors"
	"image"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.viam.com/utils"

	"github.com/edaniels/gostream/codec"
	ourwebrtc "github.com/edaniels/gostream/webrtc"

	"github.com/edaniels/golog"
	"github.com/pion/webrtc/v3"
)

// A Stream is sink that accepts any image frames for the purpose
// of displaying in a WebRTC video track.
type Stream interface {
	internalStream

	Name() string

	// Start starts processing frames.
	Start()

	// Ready signals that there is at least one client connected and that
	// streams are ready for input.
	StreamingReady() <-chan struct{}

	InputFrames() chan<- FrameReleasePair

	// Stop stops further processing of frames.
	Stop()
}

type internalStream interface {
	TrackLocal() webrtc.TrackLocal
}

// FrameReleasePair associates a frame with a corresponding
// function to release its resources once the receiver of a
// pair is finished with the frame.
type FrameReleasePair struct {
	Frame   image.Image
	Release func()
}

// NewStream returns a newly configured stream that can begin to handle
// new connections.
func NewStream(config StreamConfig) (Stream, error) {
	logger := config.Logger
	if logger == nil {
		logger = Logger
	}
	if config.EncoderFactory == nil {
		return nil, errors.New("no encoder factory set")
	}
	if config.TargetFrameRate == 0 {
		config.TargetFrameRate = codec.DefaultKeyFrameInterval
	}
	ctx, cancelFunc := context.WithCancel(context.Background())

	name := config.Name
	if name == "" {
		name = uuid.NewString()
	}

	trackLocal, err := ourwebrtc.NewTrackLocalStaticSample(
		webrtc.RTPCodecCapability{MimeType: config.EncoderFactory.MIMEType()},
		name,
		name,
	)
	if err != nil {
		cancelFunc()
		return nil, err
	}

	bs := &basicStream{
		name:              name,
		config:            config,
		streamingReadyCh:  make(chan struct{}),
		trackLocal:        trackLocal,
		peerToTrack:       map[*webrtc.PeerConnection]*ourwebrtc.TrackLocalStaticSample{},
		inputChan:         make(chan FrameReleasePair),
		outputChan:        make(chan []byte),
		logger:            logger,
		shutdownCtx:       ctx,
		shutdownCtxCancel: cancelFunc,
	}

	return bs, nil
}

type basicStream struct {
	name                    string
	config                  StreamConfig
	readyOnce               sync.Once
	streamingReadyCh        chan struct{}
	trackLocal              *ourwebrtc.TrackLocalStaticSample
	peerToTrack             map[*webrtc.PeerConnection]*ourwebrtc.TrackLocalStaticSample
	inputChan               chan FrameReleasePair
	outputChan              chan []byte
	encoder                 codec.Encoder
	shutdownCtx             context.Context
	shutdownCtxCancel       func()
	activeBackgroundWorkers sync.WaitGroup
	logger                  golog.Logger
}

func (bs *basicStream) Name() string {
	return bs.name
}

func (bs *basicStream) Start() {
	bs.readyOnce.Do(func() {
		close(bs.streamingReadyCh)
		bs.activeBackgroundWorkers.Add(2)
		utils.ManagedGo(bs.processInputFrames, bs.activeBackgroundWorkers.Done)
		utils.ManagedGo(bs.processOutputFrames, bs.activeBackgroundWorkers.Done)
	})
}

func (bs *basicStream) StreamingReady() <-chan struct{} {
	return bs.streamingReadyCh
}

func (bs *basicStream) InputFrames() chan<- FrameReleasePair {
	return bs.inputChan
}

func (bs *basicStream) TrackLocal() webrtc.TrackLocal {
	return bs.trackLocal
}

func (bs *basicStream) Stop() {
	bs.shutdownCtxCancel()
	bs.activeBackgroundWorkers.Wait()
}

func (bs *basicStream) processInputFrames() {
	frameLimiterDur := time.Second / time.Duration(bs.config.TargetFrameRate)
	defer close(bs.outputChan)
	firstFrame := true
	ticker := time.NewTicker(frameLimiterDur)
	defer ticker.Stop()
	for {
		select {
		case <-bs.shutdownCtx.Done():
			return
		default:
		}
		select {
		case <-bs.shutdownCtx.Done():
			return
		case <-ticker.C:
		}
		var framePair FrameReleasePair
		select {
		case framePair = <-bs.inputChan:
		case <-bs.shutdownCtx.Done():
			return
		}
		if framePair.Frame == nil {
			continue
		}
		var initErr bool
		func() {
			if framePair.Release != nil {
				defer framePair.Release()
			}
			if firstFrame {
				bounds := framePair.Frame.Bounds()
				if err := bs.initCodec(bounds.Dx(), bounds.Dy()); err != nil {
					bs.logger.Error(err)
					initErr = true
					return
				}
				firstFrame = false
			}

			// thread-safe because the size is static
			encodedFrame, err := bs.encoder.Encode(framePair.Frame)
			if err != nil {
				bs.logger.Error(err)
				return
			}
			if encodedFrame != nil {
				bs.outputChan <- encodedFrame
			}
		}()
		if initErr {
			return
		}
	}
}

func (bs *basicStream) processOutputFrames() {
	framesSent := 0
	for outputFrame := range bs.outputChan {
		select {
		case <-bs.shutdownCtx.Done():
			return
		default:
		}
		now := time.Now()
		if err := bs.trackLocal.WriteFrame(outputFrame); err != nil {
			bs.logger.Errorw("error writing frame", "error", err)
		}
		framesSent++
		if Debug {
			bs.logger.Debugw("wrote sample", "frames_sent", framesSent, "write_time", time.Since(now))
		}
	}
}

func (bs *basicStream) initCodec(width, height int) error {
	var err error
	bs.encoder, err = bs.config.EncoderFactory.New(width, height, bs.config.TargetFrameRate, bs.logger)
	return err
}
