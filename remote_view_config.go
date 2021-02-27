package gostream

import (
	"github.com/edaniels/golog"

	"github.com/pion/webrtc/v3"
)

var (
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

// PartialDefaultRemoteViewConfig is invalid by default;
// it requires an EncoderFactory to be set.
var PartialDefaultRemoteViewConfig = RemoteViewConfig{
	StreamNumber: 0,
	WebRTCConfig: webrtc.Configuration{
		ICEServers: DefaultICEServers,
	},
}

type RemoteViewConfig struct {
	StreamNumber    int
	StreamName      string
	WebRTCConfig    webrtc.Configuration
	Debug           bool
	EncoderFactory  EncoderFactory
	TargetFrameRate int
	Logger          golog.Logger
}
