package gostream

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/edaniels/golog"

	"github.com/pion/webrtc/v3"
	"github.com/pion/webrtc/v3/pkg/media"
)

// TODO(erd): raise this back much higher after firefox hang issue fixed
const DefaultKeyFrameInterval = 1

type RemoteView interface {
	Stop()
	Ready() <-chan struct{}
	SetOnClickHandler(func(x, y int))
	SetOnDataHandler(func(data []byte))
	SendData(data []byte)
	SendText(msg string)
	Debug() bool
	HTML() RemoteViewHTML
	SinglePageHTML() string
	Handler() RemoteViewHandler
	CommandRegistry() CommandRegistry
	ReserveStream(name string) Stream
}

type Stream interface {
	InputFrames() chan<- image.Image
}

type RemoteViewHTML struct {
	JavaScript string
	Body       string
}

func NewRemoteView(config RemoteViewConfig) (RemoteView, error) {
	logger := config.Logger
	if logger == nil {
		logger = golog.Global
	}
	if config.EncoderFactory == nil {
		return nil, errors.New("no encoder factory set")
	}
	ctx, cancelFunc := context.WithCancel(context.Background())
	return &basicRemoteView{
		config:             config,
		readyCh:            make(chan struct{}),
		peerToRemoteClient: map[*webrtc.PeerConnection]remoteClient{},
		commandRegistry:    NewCommandRegistry(),
		logger:             logger,
		shutdownCtx:        ctx,
		shutdownCtxCancel:  cancelFunc,
	}, nil
}

type basicRemoteView struct {
	mu                   sync.Mutex
	config               RemoteViewConfig
	readyOnce            sync.Once
	readyCh              chan struct{}
	peerToRemoteClient   map[*webrtc.PeerConnection]remoteClient
	inoutFrameChans      []inoutFrameChan // not thread-safe
	encoders             []Encoder        // not thread-safe
	reservedStreams      []*remoteStream
	onDataHandler        func(data []byte)
	onClickHandler       func(x, y int)
	commandRegistry      CommandRegistry
	shutdownCtx          context.Context
	shutdownCtxCancel    func()
	backgroundProcessing sync.WaitGroup
	logger               golog.Logger
}

type RemoteViewHandler struct {
	Name string
	Func http.HandlerFunc
}

func (brv *basicRemoteView) streamNum() int {
	if brv.config.StreamNumber != 0 {
		return brv.config.StreamNumber
	}
	return 0
}

func (brv *basicRemoteView) CommandRegistry() CommandRegistry {
	return brv.commandRegistry
}

func (brv *basicRemoteView) Handler() RemoteViewHandler {
	handlerName := fmt.Sprintf("offer_%d", brv.streamNum())
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
		defer r.Body.Close()

		reader := bufio.NewReader(r.Body)

		var in string
		for {
			var err error
			in, err = reader.ReadString('\n')
			if err != io.EOF {
				if err != nil {
					panic(err)
				}
			}
			in = strings.TrimSpace(in)
			if len(in) > 0 {
				break
			}
		}

		offer := webrtc.SessionDescription{}
		Decode(in, &offer)

		m := webrtc.MediaEngine{}
		if err := m.RegisterDefaultCodecs(); err != nil {
			panic(err)
		}
		options := []func(a *webrtc.API){webrtc.WithMediaEngine(&m)}
		if brv.config.Debug {
			options = append(options, webrtc.WithSettingEngine(webrtc.SettingEngine{
				LoggerFactory: webrtcLoggerFactory{brv.logger},
			}))
		}
		webAPI := webrtc.NewAPI(options...)

		// Create a new RTCPeerConnection
		peerConnection, err := webAPI.NewPeerConnection(brv.config.WebRTCConfig)
		if err != nil {
			panic(err)
		}

		iceConnectedCtx, iceConnectedCtxCancel := context.WithCancel(context.TODO())

		// Set the handler for ICE connection state
		// This will notify you when the peer has connected/disconnected
		peerConnection.OnICEConnectionStateChange(func(connectionState webrtc.ICEConnectionState) {
			connInfo := getPeerConnectionStats(peerConnection)
			brv.logger.Debugw("connection state changed",
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
				brv.removeRemoteClient(peerConnection)
			}
		})

		videoTracks := make([]*webrtc.TrackLocalStaticSample, 0, brv.numReservedStreams())
		for i, stream := range brv.getReservedStreams() {
			var trackName string // shows up as stream id
			if stream.name == "" {
				trackName = fmt.Sprintf("video-%d", i)
			} else {
				trackName = stream.name
			}
			videoTrack, err := webrtc.NewTrackLocalStaticSample(
				webrtc.RTPCodecCapability{MimeType: brv.config.EncoderFactory.MIMEType()},
				trackName,
				trackName,
			)
			if err != nil {
				panic(err)
			}
			if _, err := peerConnection.AddTrack(videoTrack); err != nil {
				panic(err)
			}
			videoTracks = append(videoTracks, videoTrack)
		}

		dataChannelID := uint16(0)
		dataChannel, err := peerConnection.CreateDataChannel("data", &webrtc.DataChannelInit{
			ID: &dataChannelID,
		})
		if err != nil {
			panic(err)
		}
		dataChannel.OnMessage(func(msg webrtc.DataChannelMessage) {
			if brv.onDataHandler == nil {
				if !msg.IsString {
					return
				}
				cmd, err := UnmarshalCommand(string(msg.Data))
				if err != nil {
					brv.logger.Debugw("error unmarshaling command", "error", err)
					if err := dataChannel.SendText(err.Error()); err != nil {
						brv.logger.Error(err)
					}
					return
				}
				resp, err := brv.CommandRegistry().Process(cmd)
				if err != nil {
					brv.logger.Debugw("error processing command", "error", err)
					if err := dataChannel.SendText(err.Error()); err != nil {
						brv.logger.Error(err)
					}
					return
				}
				if resp == nil {
					return
				}
				if resp.isText {
					if err := dataChannel.SendText(string(resp.data)); err != nil {
						brv.logger.Error(err)
					}
					return
				}
				if err := dataChannel.Send(resp.data); err != nil {
					brv.logger.Error(err)
				}
			}
			brv.onDataHandler(msg.Data)
		})

		clickChannelID := uint16(1)
		clickChannel, err := peerConnection.CreateDataChannel("clicks", &webrtc.DataChannelInit{
			ID: &clickChannelID,
		})
		if err != nil {
			panic(err)
		}
		clickChannel.OnMessage(func(msg webrtc.DataChannelMessage) {
			if brv.onClickHandler == nil {
				return
			}
			coords := strings.Split(string(msg.Data), ",")
			if len(coords) != 2 {
				panic(len(coords))
			}
			x, err := strconv.ParseFloat(coords[0], 32)
			if err != nil {
				panic(err)
			}
			y, err := strconv.ParseFloat(coords[1], 32)
			if err != nil {
				panic(err)
			}
			brv.onClickHandler(int(x), int(y)) // handler should return fast otherwise it could block
		})

		// Set the remote SessionDescription
		if err := peerConnection.SetRemoteDescription(offer); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			if _, err := w.Write([]byte(err.Error())); err != nil {
				brv.logger.Error(err)
			}
			return
		}

		// Create answer
		answer, err := peerConnection.CreateAnswer(nil)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			if _, err := w.Write([]byte(err.Error())); err != nil {
				brv.logger.Error(err)
			}
			return
		}

		// Create channel that is blocked until ICE Gathering is complete
		gatherComplete := webrtc.GatheringCompletePromise(peerConnection)

		// Sets the LocalDescription, and starts our UDP listeners
		if err := peerConnection.SetLocalDescription(answer); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			if _, err := w.Write([]byte(err.Error())); err != nil {
				brv.logger.Error(err)
			}
			return
		}

		// Block until ICE Gathering is complete, disabling trickle ICE
		// we do this because we only can exchange one signaling message
		// in a production application you should exchange ICE Candidates via OnICECandidate
		select {
		case <-brv.shutdownCtx.Done():
			return
		case <-gatherComplete:
		}

		// Output the answer in base64 so we can paste it in browser
		if _, err := w.Write([]byte(Encode(*peerConnection.LocalDescription()))); err != nil {
			brv.logger.Error(err)
			return
		}

		brv.backgroundProcessing.Add(1)
		go func() {
			defer brv.backgroundProcessing.Done()
			select {
			case <-brv.shutdownCtx.Done():
				return
			case <-iceConnectedCtx.Done():
			}

			brv.addRemoteClient(peerConnection, remoteClient{dataChannel, videoTracks})

			brv.readyOnce.Do(func() {
				close(brv.readyCh)
				brv.backgroundProcessing.Add(2)
				go brv.processInputFrames()
				go brv.processOutputFrames()
			})
		}()
	})
	return RemoteViewHandler{handlerName, handlerFunc}
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

func (brv *basicRemoteView) iceServers() string {
	var strBuf bytes.Buffer
	strBuf.WriteString("[")
	for _, server := range brv.config.WebRTCConfig.ICEServers {
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
	if len(brv.config.WebRTCConfig.ICEServers) > 0 {
		strBuf.Truncate(strBuf.Len() - 1)
	}
	strBuf.WriteString("]")
	return strBuf.String()
}

func (brv *basicRemoteView) htmlArgs() []interface{} {
	name := brv.config.StreamName
	if name != "" {
		name = " " + name
	}
	return []interface{}{name, brv.streamNum(), brv.numReservedStreams(), brv.iceServers()}
}

func (brv *basicRemoteView) SinglePageHTML() string {
	return fmt.Sprintf(viewSingleHTML, brv.htmlArgs()...)
}

func (brv *basicRemoteView) HTML() RemoteViewHTML {
	return RemoteViewHTML{
		JavaScript: fmt.Sprintf(viewJS, brv.htmlArgs()...),
		Body:       fmt.Sprintf(viewBody, brv.htmlArgs()...),
	}
}

func (brv *basicRemoteView) numReservedStreams() int {
	// thread-safe but racey if more chans are added before the first
	// connection is negotiated.
	return len(brv.reservedStreams)
}

func (brv *basicRemoteView) Debug() bool {
	return brv.config.Debug
}

func (brv *basicRemoteView) Ready() <-chan struct{} {
	return brv.readyCh
}

func (brv *basicRemoteView) Stop() {
	brv.shutdownCtxCancel()
	brv.backgroundProcessing.Wait()
}

func (brv *basicRemoteView) SetOnDataHandler(handler func(data []byte)) {
	brv.mu.Lock()
	defer brv.mu.Unlock()
	brv.onDataHandler = handler
}

func (brv *basicRemoteView) SetOnClickHandler(handler func(x, y int)) {
	brv.mu.Lock()
	defer brv.mu.Unlock()
	brv.onClickHandler = handler
}

func (brv *basicRemoteView) SendData(data []byte) {
	for _, rc := range brv.getRemoteClients() {
		if err := rc.dataChannel.Send(data); err != nil {
			brv.logger.Error(err)
		}
	}
}

func (brv *basicRemoteView) SendText(msg string) {
	for _, rc := range brv.getRemoteClients() {
		if err := rc.dataChannel.SendText(msg); err != nil {
			brv.logger.Error(err)
		}
	}
}

type inoutFrameChan struct {
	In  chan image.Image
	Out chan []byte
}

func (brv *basicRemoteView) ReserveStream(name string) Stream {
	brv.mu.Lock()
	defer brv.mu.Unlock()
	inputChan := make(chan image.Image)
	outputChan := make(chan []byte)
	brv.inoutFrameChans = append(brv.inoutFrameChans, inoutFrameChan{inputChan, outputChan})
	stream := &remoteStream{strings.ReplaceAll(name, " ", "-"), inputChan}
	brv.reservedStreams = append(brv.reservedStreams, stream)
	return stream
}

type remoteStream struct {
	name        string
	inputFrames chan<- image.Image
}

func (rs *remoteStream) InputFrames() chan<- image.Image {
	return rs.inputFrames
}

// TODO(erd): support new chans over time. Right now
// only what is streamed before the first connection is
// negotiated is streamed.
func (brv *basicRemoteView) processInputFrames() {
	defer brv.backgroundProcessing.Done()
	brv.backgroundProcessing.Add(brv.numReservedStreams())
	for i, inout := range brv.inoutFrameChans {
		iCopy := i
		inoutCopy := inout
		go func() {
			i := iCopy
			inout := inoutCopy
			defer func() {
				close(inout.Out)
				defer brv.backgroundProcessing.Done()
			}()
			firstFrame := true
			for {
				select {
				case <-brv.shutdownCtx.Done():
					return
				default:
				}
				var frame image.Image
				select {
				case frame = <-inout.In:
				case <-brv.shutdownCtx.Done():
					return
				}
				if frame == nil {
					continue
				}
				if firstFrame {
					bounds := frame.Bounds()
					if err := brv.initCodec(i, bounds.Dx(), bounds.Dy()); err != nil {
						brv.logger.Error(err)
						return
					}
					firstFrame = false
				}

				// thread-safe because the size is static
				encodedFrame, err := brv.encoders[i].Encode(frame)
				if err != nil {
					brv.logger.Error(err)
					continue
				}
				if encodedFrame != nil {
					inout.Out <- encodedFrame
				}
			}
		}()
	}
}

func (brv *basicRemoteView) processOutputFrames() {
	defer brv.backgroundProcessing.Done()

	brv.backgroundProcessing.Add(brv.numReservedStreams())
	for i, inout := range brv.inoutFrameChans {
		iCopy := i
		inoutCopy := inout
		go func() {
			i := iCopy
			inout := inoutCopy
			defer brv.backgroundProcessing.Done()

			framesSent := 0
			lastTimestamp := time.Now()
			for outputFrame := range inout.Out {
				select {
				case <-brv.shutdownCtx.Done():
					return
				default:
				}
				now := time.Now()
				duration := now.Sub(lastTimestamp)
				lastTimestamp = now
				for _, rc := range brv.getRemoteClients() {
					if ivfErr := rc.videoTracks[i].WriteSample(media.Sample{Data: outputFrame, Duration: duration}); ivfErr != nil {
						panic(ivfErr)
					}
				}
				framesSent++
				if brv.config.Debug {
					brv.logger.Debugw("wrote sample", "track_num", i, "frames_sent", framesSent, "write_time", time.Since(now))
				}
			}
		}()
	}
}

func (brv *basicRemoteView) initCodec(num, width, height int) error {
	brv.mu.Lock()
	if brv.encoders == nil {
		// this makes us stuck with this many encoders to stay thread-safe
		brv.encoders = make([]Encoder, brv.numReservedStreams())
	}
	brv.mu.Unlock()
	if brv.encoders[num] != nil {
		return errors.New("already initialized codec")
	}

	var err error
	brv.encoders[num], err = brv.config.EncoderFactory.New(width, height, brv.logger)
	return err
}

type remoteClient struct {
	dataChannel *webrtc.DataChannel
	videoTracks []*webrtc.TrackLocalStaticSample
}

func (brv *basicRemoteView) addRemoteClient(peerConnection *webrtc.PeerConnection, rc remoteClient) {
	brv.mu.Lock()
	defer brv.mu.Unlock()
	brv.peerToRemoteClient[peerConnection] = rc
}

func (brv *basicRemoteView) removeRemoteClient(peerConnection *webrtc.PeerConnection) {
	brv.mu.Lock()
	defer brv.mu.Unlock()
	delete(brv.peerToRemoteClient, peerConnection)
}

func (brv *basicRemoteView) getRemoteClients() []remoteClient {
	brv.mu.Lock()
	defer brv.mu.Unlock()
	// make shallow copy
	remoteClients := make([]remoteClient, 0, len(brv.peerToRemoteClient))
	for _, rc := range brv.peerToRemoteClient {
		remoteClients = append(remoteClients, rc)
	}
	return remoteClients
}

func (brv *basicRemoteView) getReservedStreams() []*remoteStream {
	brv.mu.Lock()
	defer brv.mu.Unlock()
	// make shallow copy
	streams := make([]*remoteStream, 0, len(brv.reservedStreams))
	return append(streams, brv.reservedStreams...)
}
