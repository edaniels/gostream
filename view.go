package gostream

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/edaniels/gostream/codec"
	ourwebrtc "github.com/edaniels/gostream/webrtc"

	"github.com/edaniels/golog"
	"github.com/pion/interceptor"
	"github.com/pion/webrtc/v3"
)

// A View represents a view of streams that can be communicated with
// from a client.
type View interface {
	// Stop stops further processing of streams and will not accept new
	// connections.
	Stop()
	// Ready signals that there is at least one client connected and that
	// streams are ready for input.
	StreamingReady() <-chan struct{}
	// SetOnClickHandler sets a handler for clicks on the view. This is typically
	// used to alter the view or send information back with SendDataToAll or SendTextToAll.
	SetOnClickHandler(func(x, y int, responder ClientResponder))
	// SetOnDataHandler sets a handler for data sent to the view. This is typically
	// used to alter the view or send information back with SendDataToAll or SendTextToAll. For
	// higher level processing of data, prefer the CommandRegistry.
	SetOnDataHandler(func(data []byte, responder ClientResponder))
	// SendDataToAll allows sending arbitrary data to all clients.
	SendDataToAll(data []byte)
	// SendTextToAll allows sending arbitrary messages to all clients.
	SendTextToAll(msg string)
	// HTML returns the HTML needed to interact with the view in a browser.
	HTML() ViewHTML
	// SinglePageHTML returns a complete HTML document that can interact with the view in a browser.
	SinglePageHTML() string
	// Handler returns a named http.Handler that handles new WebRTC connections.
	Handler() ViewHandler
	// CommandRegistry returns the command registry associated with this view. This is helpful
	// for high level interactions with the view and implementing application.
	CommandRegistry() CommandRegistry
	// ReserveStream allocates a Stream of the given name to be able to stream images to. Reserving
	// streams after the Ready signal is fired is currently not allowed but the method will not fail.
	// This is a lower level method and typically StreamSource is used instead.
	ReserveStream(name string) Stream
}

// NewView returns a newly configured view that can begin to handle
// new connections.
func NewView(config ViewConfig) (View, error) {
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
	return &basicView{
		config:             config,
		streamingReadyCh:   make(chan struct{}),
		peerToRemoteClient: map[*webrtc.PeerConnection]remoteClient{},
		commandRegistry:    NewCommandRegistry(),
		logger:             logger,
		shutdownCtx:        ctx,
		shutdownCtxCancel:  cancelFunc,
	}, nil
}

type basicView struct {
	mu                   sync.Mutex
	config               ViewConfig
	readyOnce            sync.Once
	streamingReadyCh     chan struct{}
	peerToRemoteClient   map[*webrtc.PeerConnection]remoteClient
	inoutFrameChans      []inoutFrameChan // not thread-safe
	encoders             []codec.Encoder  // not thread-safe
	reservedStreams      []*remoteStream
	onDataHandler        func(data []byte, responder ClientResponder)
	onClickHandler       func(x, y int, responder ClientResponder)
	commandRegistry      CommandRegistry
	shutdownCtx          context.Context
	shutdownCtxCancel    func()
	backgroundProcessing sync.WaitGroup
	logger               golog.Logger
}

// A ClientResponder is able to respond directly to a client. This
// is used in the onData/Click handlers.
type ClientResponder interface {
	Send(data []byte)
	SendText(s string)
}

type dataChannelClientResponder struct {
	dc     *webrtc.DataChannel
	logger golog.Logger
}

func (r dataChannelClientResponder) Send(data []byte) {
	if err := r.dc.Send(data); err != nil {
		r.logger.Error(err)
	}
}

func (r dataChannelClientResponder) SendText(s string) {
	if err := r.dc.SendText(s); err != nil {
		r.logger.Error(err)
	}
}

// A ViewHandler names a view and its http.Handler for processing
// new WebRTC connections.
type ViewHandler struct {
	Name string
	Func http.HandlerFunc
}

func (bv *basicView) streamNum() int {
	if bv.config.StreamNumber != 0 {
		return bv.config.StreamNumber
	}
	return 0
}

func (bv *basicView) CommandRegistry() CommandRegistry {
	return bv.commandRegistry
}

// TODO(erd): this method is insanely long, refactor it into some readable/manageable
// bits.
func (bv *basicView) handleOffer(w io.Writer, r *http.Request) error {
	reader := bufio.NewReader(r.Body)

	var in string
	for {
		var err error
		in, err = reader.ReadString('\n')
		if err != io.EOF {
			if err != nil {
				return err
			}
		}
		in = strings.TrimSpace(in)
		if len(in) > 0 {
			break
		}
	}

	offer := webrtc.SessionDescription{}
	if err := ourwebrtc.DecodeSDP(in, &offer); err != nil {
		return err
	}

	m := webrtc.MediaEngine{}
	if err := m.RegisterDefaultCodecs(); err != nil {
		return err
	}
	i := interceptor.Registry{}
	if err := webrtc.RegisterDefaultInterceptors(&m, &i); err != nil {
		return err
	}

	options := []func(a *webrtc.API){webrtc.WithMediaEngine(&m), webrtc.WithInterceptorRegistry(&i)}
	if Debug {
		options = append(options, webrtc.WithSettingEngine(webrtc.SettingEngine{
			LoggerFactory: ourwebrtc.LoggerFactory{bv.logger},
		}))
	}
	webAPI := webrtc.NewAPI(options...)

	// Create a new RTCPeerConnection
	peerConnection, err := webAPI.NewPeerConnection(bv.config.WebRTCConfig)
	if err != nil {
		return err
	}

	iceConnectedCtx, iceConnectedCtxCancel := context.WithCancel(context.TODO())

	// Set the handler for ICE connection state
	// This will notify you when the peer has connected/disconnected
	peerConnection.OnICEConnectionStateChange(func(connectionState webrtc.ICEConnectionState) {
		connInfo := getPeerConnectionStats(peerConnection)
		bv.logger.Debugw("connection state changed",
			"conn_id", connInfo.ID,
			"conn_state", connectionState.String(),
			"conn_remote_candidates", connInfo.RemoteCandidates,
		)
		if connectionState == webrtc.ICEConnectionStateConnected {
			iceConnectedCtxCancel()
			return
		}
		switch connectionState {
		case webrtc.ICEConnectionStateDisconnected,
			webrtc.ICEConnectionStateFailed,
			webrtc.ICEConnectionStateClosed:
			bv.removeRemoteClient(peerConnection)
		}
	})

	videoTracks := make([]*ourwebrtc.TrackLocalStaticSample, 0, bv.numReservedStreams())
	for i, stream := range bv.getReservedStreams() {
		var trackName string // shows up as stream id
		if stream.name == "" {
			trackName = fmt.Sprintf("video-%d", i)
		} else {
			trackName = stream.name
		}
		videoTrack, err := ourwebrtc.NewTrackLocalStaticSample(
			webrtc.RTPCodecCapability{MimeType: bv.config.EncoderFactory.MIMEType()},
			trackName,
			trackName,
		)
		if err != nil {
			return err
		}
		if _, err := peerConnection.AddTrack(videoTrack); err != nil {
			return err
		}
		videoTracks = append(videoTracks, videoTrack)
	}

	dataChannelID := uint16(0)
	dataChannel, err := peerConnection.CreateDataChannel("data", &webrtc.DataChannelInit{
		ID: &dataChannelID,
	})
	if err != nil {
		return err
	}
	dataChannel.OnMessage(func(msg webrtc.DataChannelMessage) {
		if bv.onDataHandler == nil {
			if !msg.IsString {
				return
			}
			cmd, err := UnmarshalCommand(string(msg.Data))
			if err != nil {
				bv.logger.Debugw("error unmarshaling command", "error", err)
				if err := dataChannel.SendText(err.Error()); err != nil {
					bv.logger.Error(err)
				}
				return
			}
			resp, err := bv.CommandRegistry().Process(cmd)
			if err != nil {
				bv.logger.Debugw("error processing command", "error", err)
				if err := dataChannel.SendText(err.Error()); err != nil {
					bv.logger.Error(err)
				}
				return
			}
			if resp == nil {
				return
			}
			if resp.isText {
				if err := dataChannel.SendText(string(resp.data)); err != nil {
					bv.logger.Error(err)
				}
				return
			}
			if err := dataChannel.Send(resp.data); err != nil {
				bv.logger.Error(err)
			}
		}
		bv.onDataHandler(msg.Data, dataChannelClientResponder{dataChannel, bv.logger})
	})

	clickChannelID := uint16(1)
	clickChannel, err := peerConnection.CreateDataChannel("clicks", &webrtc.DataChannelInit{
		ID: &clickChannelID,
	})
	if err != nil {
		return err
	}
	clickChannel.OnMessage(func(msg webrtc.DataChannelMessage) {
		if bv.onClickHandler == nil {
			return
		}
		coords := strings.Split(string(msg.Data), ",")
		if len(coords) != 2 {
			bv.logger.Debug("malformed coords")
			return
		}
		x, err := strconv.ParseFloat(coords[0], 32)
		if err != nil {
			bv.logger.Debugw("error parsing coords", "error", err)
			return
		}
		y, err := strconv.ParseFloat(coords[1], 32)
		if err != nil {
			bv.logger.Debugw("error parsing coords", "error", err)
			return
		}
		// handler should return fast otherwise it could block
		bv.onClickHandler(int(x), int(y), dataChannelClientResponder{dataChannel, bv.logger})
	})

	// Set the remote SessionDescription
	if err := peerConnection.SetRemoteDescription(offer); err != nil {
		return err
	}

	// Create answer
	answer, err := peerConnection.CreateAnswer(nil)
	if err != nil {
		return err
	}

	// Create channel that is blocked until ICE Gathering is complete
	gatherComplete := webrtc.GatheringCompletePromise(peerConnection)

	// Sets the LocalDescription, and starts our UDP listeners
	if err := peerConnection.SetLocalDescription(answer); err != nil {
		return err
	}

	// Block until ICE Gathering is complete, disabling trickle ICE
	// we do this because we only can exchange one signaling message
	// in a production application you should exchange ICE Candidates via OnICECandidate
	select {
	case <-bv.shutdownCtx.Done():
		return bv.shutdownCtx.Err()
	case <-gatherComplete:
	}

	encodedSDP, err := ourwebrtc.EncodeSDP(peerConnection.LocalDescription())
	if err != nil {
		return err
	}
	if _, err := w.Write([]byte(encodedSDP)); err != nil {
		return err
	}

	bv.backgroundProcessing.Add(1)
	go func() {
		defer bv.backgroundProcessing.Done()
		select {
		case <-bv.shutdownCtx.Done():
			return
		case <-iceConnectedCtx.Done():
		}

		bv.addRemoteClient(peerConnection, remoteClient{dataChannel, videoTracks})

		bv.readyOnce.Do(func() {
			close(bv.streamingReadyCh)
			bv.backgroundProcessing.Add(2)
			go bv.processInputFrames()
			go bv.processOutputFrames()
		})
	}()

	return nil
}

func (bv *basicView) Handler() ViewHandler {
	handlerName := fmt.Sprintf("offer_%d", bv.streamNum())
	handlerFunc := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
			w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization")
			return
		}
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		defer func() {
			if err := r.Body.Close(); err != nil {
				Logger.Debugw("error closing body", "error", err)
			}
		}()
		if err := bv.handleOffer(w, r); err != nil {
			bv.logger.Debugw("error handling offer", "error", err)
			w.WriteHeader(http.StatusInternalServerError)
			if _, err := w.Write([]byte(err.Error())); err != nil {
				bv.logger.Error(err)
			}
		}
	})
	return ViewHandler{handlerName, handlerFunc}
}

type peerConnectionStats struct {
	ID               string
	RemoteCandidates map[string]string
}

func getPeerConnectionStats(peerConnection *webrtc.PeerConnection) peerConnectionStats {
	stats := peerConnection.GetStats()
	var connID string
	connInfo := map[string]string{}
	for _, stat := range stats {
		if pcStats, ok := stat.(webrtc.PeerConnectionStats); ok {
			connID = pcStats.ID
		}
		candidateStats, ok := stat.(webrtc.ICECandidateStats)
		if !ok {
			continue
		}
		if candidateStats.Type != webrtc.StatsTypeRemoteCandidate {
			continue
		}
		var candidateType string
		switch candidateStats.CandidateType {
		case webrtc.ICECandidateTypeRelay:
			candidateType = "relay"
		case webrtc.ICECandidateTypePrflx:
			candidateType = "peer-reflexive"
		case webrtc.ICECandidateTypeSrflx:
			candidateType = "server-reflexive"
		}
		if candidateType == "" {
			continue
		}
		connInfo[candidateType] = candidateStats.IP
	}
	return peerConnectionStats{connID, connInfo}
}

func (bv *basicView) iceServers() string {
	var strBuf bytes.Buffer
	strBuf.WriteString("[")
	for _, server := range bv.config.WebRTCConfig.ICEServers {
		strBuf.WriteString("{")
		strBuf.WriteString("urls: ['")
		for _, u := range server.URLs {
			strBuf.WriteString(u)
			strBuf.WriteString("',")
		}
		if len(server.URLs) > 0 {
			strBuf.Truncate(strBuf.Len() - 1)
		}
		strBuf.WriteString("]")
		if server.Username != "" {
			strBuf.WriteString(",username:'")
			strBuf.WriteString(server.Username)
			strBuf.WriteString("'")
		}
		if cred, ok := server.Credential.(string); ok {
			strBuf.WriteString(",credential:'")
			strBuf.WriteString(cred)
			strBuf.WriteString("'")
		}
		strBuf.WriteString("},")
	}
	if len(bv.config.WebRTCConfig.ICEServers) > 0 {
		strBuf.Truncate(strBuf.Len() - 1)
	}
	strBuf.WriteString("]")
	return strBuf.String()
}

func (bv *basicView) htmlArgs() []interface{} {
	name := bv.config.StreamName
	if name != "" {
		name = " " + name
	}
	return []interface{}{name, bv.streamNum(), bv.numReservedStreams(), bv.iceServers()}
}

func (bv *basicView) numReservedStreams() int {
	// thread-safe but racey if more chans are added before the first
	// connection is negotiated.
	return len(bv.reservedStreams)
}

func (bv *basicView) StreamingReady() <-chan struct{} {
	return bv.streamingReadyCh
}

func (bv *basicView) Stop() {
	bv.shutdownCtxCancel()
	bv.backgroundProcessing.Wait()
}

func (bv *basicView) SetOnDataHandler(handler func(data []byte, responder ClientResponder)) {
	bv.mu.Lock()
	defer bv.mu.Unlock()
	bv.onDataHandler = handler
}

func (bv *basicView) SetOnClickHandler(handler func(x, y int, responder ClientResponder)) {
	bv.mu.Lock()
	defer bv.mu.Unlock()
	bv.onClickHandler = handler
}

func (bv *basicView) SendDataToAll(data []byte) {
	for _, rc := range bv.getRemoteClients() {
		if err := rc.dataChannel.Send(data); err != nil {
			bv.logger.Error(err)
		}
	}
}

func (bv *basicView) SendTextToAll(msg string) {
	for _, rc := range bv.getRemoteClients() {
		if err := rc.dataChannel.SendText(msg); err != nil {
			bv.logger.Error(err)
		}
	}
}

type inoutFrameChan struct {
	In  chan FrameReleasePair
	Out chan []byte
}

func (bv *basicView) ReserveStream(name string) Stream {
	bv.mu.Lock()
	defer bv.mu.Unlock()
	inputChan := make(chan FrameReleasePair)
	outputChan := make(chan []byte)
	bv.inoutFrameChans = append(bv.inoutFrameChans, inoutFrameChan{inputChan, outputChan})
	stream := &remoteStream{strings.ReplaceAll(name, " ", "-"), inputChan}
	bv.reservedStreams = append(bv.reservedStreams, stream)
	return stream
}

type remoteStream struct {
	name        string
	inputFrames chan<- FrameReleasePair
}

func (rs *remoteStream) InputFrames() chan<- FrameReleasePair {
	return rs.inputFrames
}

// TODO(erd): support new chans over time. Right now
// only what is streamed before the first connection is
// negotiated is streamed.
func (bv *basicView) processInputFrames() {
	defer bv.backgroundProcessing.Done()
	bv.backgroundProcessing.Add(bv.numReservedStreams())
	frameLimiterDur := time.Second / time.Duration(bv.config.TargetFrameRate)
	for i, inout := range bv.inoutFrameChans {
		iCopy := i
		inoutCopy := inout
		go func() {
			i := iCopy
			inout := inoutCopy
			defer func() {
				close(inout.Out)
				defer bv.backgroundProcessing.Done()
			}()
			firstFrame := true
			ticker := time.NewTicker(frameLimiterDur)
			defer ticker.Stop()
			for {
				select {
				case <-bv.shutdownCtx.Done():
					return
				default:
				}
				select {
				case <-bv.shutdownCtx.Done():
					return
				case <-ticker.C:
				}
				var framePair FrameReleasePair
				select {
				case framePair = <-inout.In:
				case <-bv.shutdownCtx.Done():
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
						if err := bv.initCodec(i, bounds.Dx(), bounds.Dy()); err != nil {
							bv.logger.Error(err)
							initErr = true
							return
						}
						firstFrame = false
					}

					// thread-safe because the size is static
					encodedFrame, err := bv.encoders[i].Encode(framePair.Frame)
					if err != nil {
						bv.logger.Error(err)
						return
					}
					if encodedFrame != nil {
						inout.Out <- encodedFrame
					}
				}()
				if initErr {
					return
				}
			}
		}()
	}
}

func (bv *basicView) processOutputFrames() {
	defer bv.backgroundProcessing.Done()

	bv.backgroundProcessing.Add(bv.numReservedStreams())
	for i, inout := range bv.inoutFrameChans {
		iCopy := i
		inoutCopy := inout
		go func() {
			i := iCopy
			inout := inoutCopy
			defer bv.backgroundProcessing.Done()

			framesSent := 0
			for outputFrame := range inout.Out {
				select {
				case <-bv.shutdownCtx.Done():
					return
				default:
				}
				now := time.Now()
				for _, rc := range bv.getRemoteClients() {
					if err := rc.videoTracks[i].WriteFrame(outputFrame); err != nil {
						bv.logger.Errorw("error writing frame", "error", err)
					}
				}
				framesSent++
				if Debug {
					bv.logger.Debugw("wrote sample", "track_num", i, "frames_sent", framesSent, "write_time", time.Since(now))
				}
			}
		}()
	}
}

func (bv *basicView) initCodec(num, width, height int) error {
	bv.mu.Lock()
	if bv.encoders == nil {
		// this makes us stuck with this many encoders to stay thread-safe
		bv.encoders = make([]codec.Encoder, bv.numReservedStreams())
	}
	bv.mu.Unlock()
	if bv.encoders[num] != nil {
		return errors.New("already initialized codec")
	}

	var err error
	bv.encoders[num], err = bv.config.EncoderFactory.New(width, height, bv.config.TargetFrameRate, bv.logger)
	return err
}

type remoteClient struct {
	dataChannel *webrtc.DataChannel
	videoTracks []*ourwebrtc.TrackLocalStaticSample
}

func (bv *basicView) addRemoteClient(peerConnection *webrtc.PeerConnection, rc remoteClient) {
	bv.mu.Lock()
	defer bv.mu.Unlock()
	bv.peerToRemoteClient[peerConnection] = rc
}

func (bv *basicView) removeRemoteClient(peerConnection *webrtc.PeerConnection) {
	bv.mu.Lock()
	defer bv.mu.Unlock()
	delete(bv.peerToRemoteClient, peerConnection)
}

func (bv *basicView) getRemoteClients() []remoteClient {
	bv.mu.Lock()
	defer bv.mu.Unlock()
	// make shallow copy
	remoteClients := make([]remoteClient, 0, len(bv.peerToRemoteClient))
	for _, rc := range bv.peerToRemoteClient {
		remoteClients = append(remoteClients, rc)
	}
	return remoteClients
}

func (bv *basicView) getReservedStreams() []*remoteStream {
	bv.mu.Lock()
	defer bv.mu.Unlock()
	// make shallow copy
	streams := make([]*remoteStream, 0, len(bv.reservedStreams))
	return append(streams, bv.reservedStreams...)
}
