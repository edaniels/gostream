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

	// register screen drivers.
	_ "github.com/pion/mediadevices/pkg/driver/microphone"
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

	InputImageFrames() (chan<- FrameReleasePair, error)

	InputAudioChunks(latency time.Duration) (chan<- AudioChunkReleasePair, error)

	// Stop stops further processing of frames.
	Stop()
}

type internalStream interface {
	VideoTrackLocal() (webrtc.TrackLocal, bool)
	AudioTrackLocal() (webrtc.TrackLocal, bool)
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
	if config.VideoEncoderFactory == nil && config.AudioEncoderFactory == nil {
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

	var trackLocal *ourwebrtc.TrackLocalStaticSample
	if config.VideoEncoderFactory != nil {
		trackLocal = ourwebrtc.NewTrackLocalStaticSample(
			webrtc.RTPCodecCapability{MimeType: config.VideoEncoderFactory.MIMEType()},
			name,
			name,
		)
	}

	var audioTrackLocal *ourwebrtc.TrackLocalStaticSample
	if config.AudioEncoderFactory != nil {
		audioTrackLocal = ourwebrtc.NewAudioTrackLocalStaticSample(
			webrtc.RTPCodecCapability{MimeType: config.AudioEncoderFactory.MIMEType()},
			name,
			name,
		)
	}

	bs := &basicStream{
		name:             name,
		config:           config,
		streamingReadyCh: make(chan struct{}),

		videoTrackLocal: trackLocal,
		inputImageChan:  make(chan FrameReleasePair),
		outputVideoChan: make(chan []byte),

		audioTrackLocal: audioTrackLocal,
		inputAudioChan:  make(chan AudioChunkReleasePair),
		outputAudioChan: make(chan []byte),

		logger:            logger,
		shutdownCtx:       ctx,
		shutdownCtxCancel: cancelFunc,
	}

	return bs, nil
}

type basicStream struct {
	mu               sync.Mutex
	name             string
	config           StreamConfig
	readyOnce        sync.Once
	streamingReadyCh chan struct{}

	videoTrackLocal *ourwebrtc.TrackLocalStaticSample
	inputImageChan  chan FrameReleasePair
	outputVideoChan chan []byte
	videoEncoder    codec.VideoEncoder

	audioTrackLocal *ourwebrtc.TrackLocalStaticSample
	inputAudioChan  chan AudioChunkReleasePair
	outputAudioChan chan []byte
	audioEncoder    codec.AudioEncoder

	// audioLatency specifies how long in between audio samples. This must be guaranteed
	// by all streamed audio.
	audioLatency    time.Duration
	audioLatencySet bool

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
		bs.activeBackgroundWorkers.Add(4)
		utils.ManagedGo(bs.processInputFrames, bs.activeBackgroundWorkers.Done)
		utils.ManagedGo(bs.processOutputFrames, bs.activeBackgroundWorkers.Done)
		utils.ManagedGo(bs.processInputAudioChunks, bs.activeBackgroundWorkers.Done)
		utils.ManagedGo(bs.processOutputAudioChunks, bs.activeBackgroundWorkers.Done)
	})
}

func (bs *basicStream) StreamingReady() <-chan struct{} {
	return bs.streamingReadyCh
}

func (bs *basicStream) InputImageFrames() (chan<- FrameReleasePair, error) {
	if bs.config.VideoEncoderFactory == nil {
		return nil, errors.New("no video in stream")
	}
	return bs.inputImageChan, nil
}

func (bs *basicStream) InputAudioChunks(latency time.Duration) (chan<- AudioChunkReleasePair, error) {
	if bs.config.AudioEncoderFactory == nil {
		return nil, errors.New("no audio in stream")
	}
	bs.mu.Lock()
	if bs.audioLatencySet && bs.audioLatency != latency {
		return nil, errors.New("cannot stream audio source with different latencies")
	}
	bs.audioLatencySet = true
	bs.audioLatency = latency
	bs.mu.Unlock()
	return bs.inputAudioChan, nil
}

func (bs *basicStream) VideoTrackLocal() (webrtc.TrackLocal, bool) {
	return bs.videoTrackLocal, bs.videoTrackLocal != nil
}

func (bs *basicStream) AudioTrackLocal() (webrtc.TrackLocal, bool) {
	return bs.audioTrackLocal, bs.audioTrackLocal != nil
}

func (bs *basicStream) Stop() {
	bs.shutdownCtxCancel()
	bs.activeBackgroundWorkers.Wait()
	if bs.audioEncoder != nil {
		bs.audioEncoder.Close()
	}
}

func (bs *basicStream) processInputFrames() {
	frameLimiterDur := time.Second / time.Duration(bs.config.TargetFrameRate)
	defer close(bs.outputVideoChan)
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
		case framePair = <-bs.inputImageChan:
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

				if err := bs.initVideoCodec(dx, dy); err != nil {
					bs.logger.Error(err)
					initErr = true
					return
				}
			}

			// thread-safe because the size is static
			encodedFrame, err := bs.videoEncoder.Encode(bs.shutdownCtx, framePair.Frame)
			if err != nil {
				bs.logger.Error(err)
				return
			}
			if encodedFrame != nil {
				select {
				case <-bs.shutdownCtx.Done():
					return
				case bs.outputVideoChan <- encodedFrame:
				}
			}
		}()
		if initErr {
			return
		}
	}
}

func (bs *basicStream) processInputAudioChunks() {
	defer close(bs.outputAudioChan)
	var samplingRate, channels int
	for {
		select {
		case <-bs.shutdownCtx.Done():
			return
		default:
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

			info := audioChunkPair.Chunk.ChunkInfo()
			newSamplingRate, newChannels := info.SamplingRate, info.Channels
			if samplingRate != newSamplingRate || channels != newChannels {
				samplingRate, channels = newSamplingRate, newChannels
				bs.logger.Infow("detected new audio info", "sampling_rate", samplingRate, "channels", channels)

				bs.audioTrackLocal.SetAudioLatency(bs.audioLatency)
				if err := bs.initAudioCodec(samplingRate, channels); err != nil {
					bs.logger.Error(err)
					initErr = true
					return
				}
			}

			encodedChunk, ready, err := bs.audioEncoder.Encode(bs.shutdownCtx, audioChunkPair.Chunk)
			if err != nil {
				bs.logger.Error(err)
				return
			}
			if ready && encodedChunk != nil {
				select {
				case <-bs.shutdownCtx.Done():
					return
				case bs.outputAudioChan <- encodedChunk:
				}
			}
		}()
		if initErr {
			return
		}
	}
}

func (bs *basicStream) processOutputFrames() {
	framesSent := 0
	for outputFrame := range bs.outputVideoChan {
		select {
		case <-bs.shutdownCtx.Done():
			return
		default:
		}
		now := time.Now()
		if err := bs.videoTrackLocal.WriteData(outputFrame); err != nil {
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
		if err := bs.audioTrackLocal.WriteData(outputChunk); err != nil {
			bs.logger.Errorw("error writing audio chunk", "error", err)
		}
		framesSent++
		if Debug {
			bs.logger.Debugw("wrote sample", "frames_sent", framesSent, "write_time", time.Since(now))
		}
	}
}

func (bs *basicStream) initVideoCodec(width, height int) error {
	var err error
	bs.videoEncoder, err = bs.config.VideoEncoderFactory.New(width, height, bs.config.TargetFrameRate, bs.logger)
	return err
}

func (bs *basicStream) initAudioCodec(sampleRate, channelCount int) error {
	var err error
	if bs.audioEncoder != nil {
		bs.audioEncoder.Close()
	}
	bs.audioEncoder, err = bs.config.AudioEncoderFactory.New(sampleRate, channelCount, bs.audioLatency, bs.logger)
	return err
}
