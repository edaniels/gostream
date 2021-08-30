package gostream

import "embed"

//go:embed assets/webrtc.html
var mainHTML string

//go:embed assets/static/core/*
var coreFS embed.FS
