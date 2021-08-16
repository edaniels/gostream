package gostream

import "embed"

//go:embed assets/index.html
var mainHTML string

//go:embed assets/static/app/*
var appFS embed.FS

//go:embed assets/static/core/*
var coreFS embed.FS

//go:embed assets/static/vendor/*
var vendorFS embed.FS
