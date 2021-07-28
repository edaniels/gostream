package macos

import "C"

import (
	"image"
	"sync"
)

type CursorHandle struct {
	mux sync.Mutex
}

type UpdateCallback func(img image.Image, width int, height int, hotx int, hoty int)

func NewCursorHandle() *CursorHandle {
	h := CursorHandle{}
	return &h
}

func (h *CursorHandle) Start(callback UpdateCallback) chan struct{} {
	quit := make(chan struct{})
	return quit
}
