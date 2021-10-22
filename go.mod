module github.com/edaniels/gostream

go 1.15

require (
	github.com/disintegration/imaging v1.6.2
	github.com/edaniels/golinters v0.0.4
	github.com/edaniels/golog v0.0.0-20210326173913-16d408aa7a5e
	github.com/golangci/golangci-lint v1.38.0
	github.com/pion/interceptor v0.1.0
	github.com/pion/logging v0.2.2
	github.com/pion/mediadevices v0.2.0
	github.com/pion/rtp v1.7.2
	github.com/pion/webrtc/v3 v3.1.5
	go.uber.org/atomic v1.9.0 // indirect
	go.uber.org/multierr v1.7.0
	go.uber.org/zap v1.19.1
	golang.org/x/net v0.0.0-20211020060615-d418f374d309 // indirect
	golang.org/x/sys v0.0.0-20211020174200-9d6173849985 // indirect
)

replace github.com/pion/mediadevices => github.com/edaniels/mediadevices v0.0.0-20211022001911-e8e6d6110b1b
