# gostream

gostream is a library to simplify the streaming of images as video to a series of WebRTC peers. The impetus for this existing was for doing simple GUI / video streaming to a browser all within go with as little cgo as possible. The package will likely be refactored over time to support some more generalized use cases and as such will be in version 0 for the time being. Many parameters are hard coded and need to be configurable over time. Use at your own risk, and please file issues!

<p align="center">
  <a href="https://pkg.go.dev/github.com/edaniels/gostream"><img src="https://pkg.go.dev/badge/github.com/edaniels/gostream" alt="PkgGoDev"></a>
  <a href="https://goreportcard.com/report/github.com/edaniels/gostream"><img src="https://goreportcard.com/badge/github.com/edaniels/gostream" alt="Go Report Card"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/License-MIT-yellow.svg" alt="License: MIT"></a>
</p>
<br>

## TODO

- Support multiple codecs (e.g. Firefox macos-arm does not support h264 by default yet)
- Verify Windows Logitech StreamCam working
- Reconnect on server restart
- Check closes and frees
- Upgrade Pion and see if Firefox stutter still present
- Address code TODOs (including context.TODO)
- Documentation (inner func docs, package docs, example docs)
- Version 0.1.0
- Tests (and integrate to GitHub Actions)

## Future Work

- Sound streaming (with video synchronization)

## Examples

* Stream current desktop: `go run github.com/edaniels/gostream/cmd/stream_desktop`

## Notes

### Firefox freezing

For some reason unknown yet, Firefox appears to freeze frequently. This is very unlikely an issue with pion/webrtc and very likely an issue with this library based on current testing. Current thoughts are that it has to do with the timing of writes causing dropped/faulty packets from the perspective of Firefox. Chromium seems to work just fine. Running Firefox with --MOZ_LOG=webrtc_trace:5 can show the errors it is seeing. There are a lot of no GoP in frame drops as well as NACK frame drops. Also curious if the issue is made worse with larger frames and different frame formats.

### Using mDNS

* mDNS (.local addresses) don't seem to work well with WebRTC yet. Random STUN/TURN failures appear to occur. At your own risk, you can address this in Firefox in `about:config` with `media.peerconnection.ice.obfuscate_host_addresses` set to `false` and in Chrome with `chrome://flags/#enable-webrtc-hide-local-ips-with-mdns` set to `Disabled`.

## Building

### Prerequisites

* libvpx

Linux: `libvpx-dev`

macOS: `brew install libvpx`

* x264

Linux: `libx264-dev`

macOS: `brew install x264`


## Development

### Linting

```
make lint
```

### Testing

```
make test
```

## Acknowledgements

If I somehow took code from somewhere without acknowledging it here or via the go.mod, please file an issue and let me know.
