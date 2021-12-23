package gostream

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"go.viam.com/utils/rpc"

	streampb "github.com/edaniels/gostream/proto/stream/v1"
)

// A StreamServer manages a collection of streams. Streams can be
// added over time for future new connections.
type StreamServer interface {
	// ServiceServer returns a service server for gRPC.
	ServiceServer() streampb.StreamServiceServer

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

func (ss *streamServer) AddStream(stream Stream) error {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	return ss.addStream(stream)
}

func (ss *streamServer) addStream(stream Stream) error {
	streamName := stream.Name()
	if _, ok := ss.nameToStream[streamName]; ok {
		return fmt.Errorf("stream %q already registered", streamName)
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

	if _, err := pc.AddTrack(streamToAdd.TrackLocal()); err != nil {
		return nil, err
	}
	streamToAdd.Start()

	return &streampb.AddStreamResponse{}, nil
}
