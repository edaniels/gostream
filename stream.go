// Package gostream implements a simple server for serving video streams over WebRTC.
package gostream

import (
	"context"
	"errors"
	"image"
	"sync"
	"time"

	"github.com/edaniels/golog"
	"github.com/google/uuid"
	"github.com/pion/mediadevices/pkg/wave"
	"github.com/pion/webrtc/v3"
	"go.viam.com/utils"

	"github.com/edaniels/gostream/codec"
	ourwebrtc "github.com/edaniels/gostream/webrtc"
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

	InputAudioChunks() chan<- AudioChunkReleasePair

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

// AudioChunkReleasePair associates an audio chunk with a corresponding
// function to release its resources once the receiver of a
// pair is finished with the chunk.
type AudioChunkReleasePair struct {
	Chunk   wave.Audio
	Release func()
}

// NewStream returns a newly configured stream that can begin to handle
// new connections.
func NewStream(config StreamConfig) (Stream, error) {
	logger := config.Logger
	if logger == nil {
		logger = Logger
	}
	if config.EncoderFactory == nil && config.AudioEncoderFactory == nil {
		return nil, errors.New("at least one audio or video encoder factory must be set")
	}
	if config.TargetFrameRate == 0 {
		config.TargetFrameRate = codec.DefaultKeyFrameInterval
	}
	ctx, cancelFunc := context.WithCancel(context.Background())

	name := config.Name
	if name == "" {
		name = uuid.NewString()
	}

	// TODO(erd): need an audio version??!!
	trackLocal := ourwebrtc.NewTrackLocalStaticSample(
		webrtc.RTPCodecCapability{MimeType: config.EncoderFactory.MIMEType()},
		name,
		name,
	)

	bs := &basicStream{
		name:              name,
		config:            config,
		streamingReadyCh:  make(chan struct{}),
		trackLocal:        trackLocal,
		peerToTrack:       map[*webrtc.PeerConnection]*ourwebrtc.TrackLocalStaticSample{},
		inputChan:         make(chan FrameReleasePair),
		outputChan:        make(chan []byte),
		inputAudioChan:    make(chan AudioChunkReleasePair),
		outputAudioChan:   make(chan []byte),
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
	inputAudioChan          chan AudioChunkReleasePair
	outputAudioChan         chan []byte
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
		utils.ManagedGo(bs.processInputAudioChunks, bs.activeBackgroundWorkers.Done)
		utils.ManagedGo(bs.processOutputAudioChnks, bs.activeBackgroundWorkers.Done)
	})
}

func (bs *basicStream) StreamingReady() <-chan struct{} {
	return bs.streamingReadyCh
}

func (bs *basicStream) InputFrames() chan<- FrameReleasePair {
	return bs.inputChan
}

func (bs *basicStream) InputAudioChunks() chan<- AudioChunkReleasePair {
	return bs.inputAudioChan
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
	var dx, dy int
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

			bounds := framePair.Frame.Bounds()
			newDx, newDy := bounds.Dx(), bounds.Dy()
			if dx != newDx || dy != newDy {
				dx, dy = newDx, newDy
				bs.logger.Infow("detected new image bounds", "width", dx, "height", dy)

				if err := bs.initCodec(dx, dy); err != nil {
					bs.logger.Error(err)
					initErr = true
					return
				}
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

func (bs *basicStream) processInputAudioChunks() {
	// TODO(erd): is this right? may be way too fast and have to use some kind of buffering or idk
	// TODO(erd): yeah this will probably use a lot of CPU
	sampleLimiterDur := time.Second / time.Duration(bs.config.TargetAudioSampleRate)
	defer close(bs.outputAudioChan)
	var dx, dy int
	ticker := time.NewTicker(sampleLimiterDur)
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
		var audioChunkPair AudioChunkReleasePair
		select {
		case audioChunkPair = <-bs.inputAudioChan:
		case <-bs.shutdownCtx.Done():
			return
		}
		if audioChunkPair.Chunk == nil {
			continue
		}
		var initErr bool
		func() {
			if audioChunkPair.Release != nil {
				defer audioChunkPair.Release()
			}

			bounds := framePair.Frame.Bounds()
			newDx, newDy := bounds.Dx(), bounds.Dy()
			if dx != newDx || dy != newDy {
				dx, dy = newDx, newDy
				bs.logger.Infow("detected new image bounds", "width", dx, "height", dy)

				if err := bs.initCodec(dx, dy); err != nil {
					bs.logger.Error(err)
					initErr = true
					return
				}
			}

			encodedChunk, err := bs.encoder.Encode(audioChunkPair.Chunk)
			if err != nil {
				bs.logger.Error(err)
				return
			}
			if encodedChunk != nil {
				bs.outputChan <- encodedChunk
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
		if err := bs.trackLocal.WriteData(outputFrame); err != nil {
			bs.logger.Errorw("error writing frame", "error", err)
		}
		framesSent++
		if Debug {
			bs.logger.Debugw("wrote sample", "frames_sent", framesSent, "write_time", time.Since(now))
		}
	}
}

func (bs *basicStream) processOutputAudioChunks() {
	framesSent := 0
	for outputChunk := range bs.outputAudioChan {
		select {
		case <-bs.shutdownCtx.Done():
			return
		default:
		}
		now := time.Now()
		if err := bs.trackLocal.WriteData(outputChunk); err != nil {
			bs.logger.Errorw("error writing audio chunk", "error", err)
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

func (bs *basicStream) initAudioCodec(sampleRate, channelCount int) error {
	var err error
	// TODO(erd): need target frame rate similar thing??
	bs.audioEncoder, err = bs.config.AudioEncoderFactory.New(sampleRate, channelCount, bs.logger)
	return err
}
