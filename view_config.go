package gostream

import (
	"github.com/edaniels/gostream/codec"

	"github.com/edaniels/golog"
	"github.com/pion/webrtc/v3"
	"go.viam.com/utils/rpc"
)

// PartialDefaultViewConfig is invalid by default;
// it requires an EncoderFactory to be set.
var PartialDefaultViewConfig = ViewConfig{
	StreamNumber: 0,
	WebRTCConfig: webrtc.Configuration{
		ICEServers: rpc.DefaultICEServers,
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
