package gostream

import (
	"github.com/trevor403/gostream/codec"

	"github.com/edaniels/golog"
	"github.com/pion/webrtc/v3"
)

var (
	// DefaultICEServers is the default set of ICE servers to use for WebRTC session negotiation.
	DefaultICEServers = []webrtc.ICEServer{
		// {
		// 	URLs: []string{"stun:stun.l.google.com:19302"},
		// },
		{
			URLs:           []string{"stun:stun.trevor.jp:3478", "turn:turn.trevor.jp:3478?transport=udp"},
			Username:       "trevor",
			Credential:     "redacted",
			CredentialType: webrtc.ICECredentialTypePassword,
		},
	}
)

// PartialDefaultViewConfig is invalid by default;
// it requires an EncoderFactory to be set.
var PartialDefaultViewConfig = ViewConfig{
	StreamNumber: 0,
	WebRTCConfig: webrtc.Configuration{
		ICEServers: DefaultICEServers,
	},
}

// A ViewConfig describes how a View should be managed.
type ViewConfig struct {
	StreamNumber   int
	StreamName     string
	WebRTCConfig   webrtc.Configuration
	EncoderFactory codec.EncoderFactory
	// TargetFrameRate will hint to the View to try to maintain this frame rate.
	TargetFrameRate int
	Logger          golog.Logger
}
