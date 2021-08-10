// +build !darwin

package platform

import "C"

import (
	"sync"
)

type CursorHandle struct {
	mux sync.Mutex
}

func NewCursorHandle() *CursorHandle {
	h := CursorHandle{}
	return &h
}

func (h *CursorHandle) SetCallback(callback UpdateCallback) {
}

func (h *CursorHandle) UpdateScale(factor float32) {
}

func (h *CursorHandle) Start() chan struct{} {
	quit := make(chan struct{})
	return quit
}
