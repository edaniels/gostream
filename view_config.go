package gostream

import (
	"github.com/edaniels/gostream/codec"

	"github.com/edaniels/golog"
	"github.com/pion/webrtc/v3"
)

var (
	// DefaultICEServers is the default set of ICE servers to use for WebRTC session negotiation.
	// There is no guarantee that the defaults here will remain usable.
	DefaultICEServers = []webrtc.ICEServer{
		// feel free to use your own ICE servers;
		// the provided is a basic coturn service with no guarantees :)
		// screen turnserver -vvvv -L 0.0.0.0 -J "mongodb://localhost" -r default -a -X "54.164.16.193/172.31.31.242"
		{
			URLs: []string{"stun:stun.erdaniels.com"},
		},
		{
			URLs:           []string{"turn:stun.erdaniels.com"},
			Username:       "username",
			Credential:     "password",
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
