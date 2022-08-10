package gostream

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"

	"go.viam.com/utils/rpc"

	streampb "github.com/edaniels/gostream/proto/stream/v1"
	"github.com/gen2brain/malgo"
	"github.com/pion/webrtc/v3"
	"gopkg.in/hraban/opus.v2"
)

// StreamAlreadyRegisteredError indicates that a stream has a name that is already registered on
// the stream server.
type StreamAlreadyRegisteredError struct {
	name string
}

func (e *StreamAlreadyRegisteredError) Error() string {
	return fmt.Sprintf("stream %q already registered", e.name)
}

// A StreamServer manages a collection of streams. Streams can be
// added over time for future new connections.
type StreamServer interface {
	// ServiceServer returns a service server for gRPC.
	ServiceServer() streampb.StreamServiceServer

	// NewStream creates a new stream from config and adds it for new connections to see.
	// Returns the added stream if it is successfully added to the server.
	NewStream(config StreamConfig) (Stream, error)

	// AddStream adds the given stream for new connections to see.
	AddStream(stream Stream) error

	// Close closes the server.
	Close() error
}

// NewStreamServer returns a server that will run on the given port and initially starts
// with the given stream.
func NewStreamServer(streams ...Stream) (StreamServer, error) {
	ss := &streamServer{nameToStream: make(map[string]Stream)}
	ss.mu.Lock()
	defer ss.mu.Unlock()
	for _, stream := range streams {
		if err := ss.addStream(stream); err != nil {
			return nil, err
		}
	}
	return ss, nil
}

type streamServer struct {
	mu           sync.RWMutex
	streams      []Stream
	nameToStream map[string]Stream
}

func (ss *streamServer) ServiceServer() streampb.StreamServiceServer {
	return &streamRPCServer{ss: ss}
}

func (ss *streamServer) NewStream(config StreamConfig) (Stream, error) {
	ss.mu.Lock()
	defer ss.mu.Unlock()

	if _, ok := ss.nameToStream[config.Name]; ok {
		return nil, &StreamAlreadyRegisteredError{config.Name}
	}
	stream, err := NewStream(config)
	if err != nil {
		return nil, err
	}
	if err := ss.addStream(stream); err != nil {
		return nil, err
	}
	return stream, nil
}

func (ss *streamServer) AddStream(stream Stream) error {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	return ss.addStream(stream)
}

func (ss *streamServer) addStream(stream Stream) error {
	streamName := stream.Name()
	if _, ok := ss.nameToStream[streamName]; ok {
		return &StreamAlreadyRegisteredError{streamName}
	}
	ss.nameToStream[streamName] = stream
	ss.streams = append(ss.streams, stream)
	return nil
}

func (ss *streamServer) Close() error {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	for _, stream := range ss.streams {
		stream.Stop()
	}
	return nil
}

type streamRPCServer struct {
	streampb.UnimplementedStreamServiceServer
	ss *streamServer
}

func (srs *streamRPCServer) ListStreams(ctx context.Context, req *streampb.ListStreamsRequest) (*streampb.ListStreamsResponse, error) {
	pc, ok := rpc.ContextPeerConnection(ctx)
	if !ok {
		return nil, errors.New("can only add a stream over a WebRTC based connection")
	}

	pc.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		switch track.Kind() {
		case webrtc.RTPCodecTypeAudio:
			{
				ctx, err := malgo.InitContext(nil, malgo.ContextConfig{}, func(message string) {
					fmt.Printf("LOG <%v>\n", message)
				})
				if err != nil {
					fmt.Println(err)
					os.Exit(1)
				}
				defer func() {
					_ = ctx.Uninit()
					ctx.Free()
				}()

				codec := track.Codec()
				channels := codec.Channels
				sampleRate := codec.ClockRate

				dec, err := opus.NewDecoder(int(sampleRate), int(channels))
				if err != nil {
					panic(err)
				}

				const maxOpusFrameSizeMs = 60
				maxFrameSize := float32(channels) * maxOpusFrameSizeMs * float32(sampleRate) / 1000
				dataPool := sync.Pool{
					New: func() interface{} {
						return make([]float32, int(maxFrameSize))
					},
				}
				decodeRPTData := func() ([]float32, int, error) {
					data, _, err := track.ReadRTP()
					if err != nil {
						return nil, 0, err
					}
					if len(data.Payload) == 0 {
						return nil, 0, nil
					}

					pcmData := dataPool.Get().([]float32)
					n, err := dec.DecodeFloat32(data.Payload, pcmData)
					return pcmData[:n*int(channels)], n, err
				}

				var periodSizeInFrames int
				for {
					// we assume all packets will contain this amount of samples going forward
					// if it's anything larger than one RTP packet (or close to MTU?) then
					// this could fail.
					_, numSamples, err := decodeRPTData()
					if err != nil {
						panic(err)
					}
					if numSamples > 0 {
						periodSizeInFrames = numSamples
						break
					}
				}

				deviceConfig := malgo.DefaultDeviceConfig(malgo.Playback)
				deviceConfig.Playback.Format = malgo.FormatF32 // tied to what we opus decode to
				deviceConfig.Playback.Channels = uint32(channels)
				deviceConfig.SampleRate = sampleRate
				deviceConfig.PeriodSizeInFrames = uint32(periodSizeInFrames)
				sizeInBytes := malgo.SampleSizeInBytes(deviceConfig.Playback.Format)

				pcmChan := make(chan []float32)

				onSendFrames := func(pOutput, _ []byte, frameCount uint32) {
					samplesRequested := frameCount * deviceConfig.Playback.Channels * uint32(sizeInBytes)
					pcm := <-pcmChan
					if len(pcm) > int(samplesRequested) {
						// logger.Errorw("not enough samples requested; trimming our own data", "samples_requested", samplesRequested)
						pcm = pcm[:samplesRequested]
					}
					pOutput = pOutput[:0]
					buf := bytes.NewBuffer(pOutput)
					binary.Write(buf, binary.LittleEndian, pcm)
					dataPool.Put(pcm)
				}

				playbackCallbacks := malgo.DeviceCallbacks{
					Data: onSendFrames,
				}

				device, err := malgo.InitDevice(ctx.Context, deviceConfig, playbackCallbacks)
				if err != nil {
					panic(err)
				}

				err = device.Start()
				if err != nil {
					panic(err)
				}

				for {
					pcmData, numSamples, err := decodeRPTData()
					if err == io.EOF {
						break
					}
					if numSamples == 0 {
						continue
					}
					pcmChan <- pcmData
				}
			}
		default:
			fmt.Println("Unsupported track.kind", track.Kind().String())
		}
	})

	srs.ss.mu.RLock()
	names := make([]string, 0, len(srs.ss.streams))
	for _, stream := range srs.ss.streams {
		names = append(names, stream.Name())
	}
	srs.ss.mu.RUnlock()
	return &streampb.ListStreamsResponse{Names: names}, nil
}

func (srs *streamRPCServer) AddStream(ctx context.Context, req *streampb.AddStreamRequest) (*streampb.AddStreamResponse, error) {
	pc, ok := rpc.ContextPeerConnection(ctx)
	if !ok {
		return nil, errors.New("can only add a stream over a WebRTC based connection")
	}

	var streamToAdd Stream
	for _, stream := range srs.ss.streams {
		if stream.Name() == req.Name {
			streamToAdd = stream
			break
		}
	}
	if streamToAdd == nil {
		return nil, fmt.Errorf("no stream for %q", req.Name)
	}

	if trackLocal, haveTrackLocal := streamToAdd.VideoTrackLocal(); haveTrackLocal {
		if _, err := pc.AddTrack(trackLocal); err != nil {
			return nil, err
		}
	}
	if trackLocal, haveTrackLocal := streamToAdd.AudioTrackLocal(); haveTrackLocal {
		if _, err := pc.AddTrack(trackLocal); err != nil {
			return nil, err
		}
	}
	streamToAdd.Start()

	return &streampb.AddStreamResponse{}, nil
}
