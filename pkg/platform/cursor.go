package platform

import (
	"image"

	"github.com/trevor403/gostream/pkg/common"
)

type UpdateCallback func(img image.Image, width int, height int, hotx int, hoty int)

type CursorHandle struct {
	callback UpdateCallback
	factor   float32
	prev     common.CursorImage
}

func NewCursorHandle() *CursorHandle {
	h := CursorHandle{}

	h.factor = 1.0

	return &h
}

func (h *CursorHandle) SetCallback(callback UpdateCallback) {
	h.callback = callback
}

func (h *CursorHandle) UpdateScale(factor float32) {
	h.factor = factor
	if h.callback == nil {
		return
	}
	if h.prev.Img == nil {
		return
	}
	cursor := h.prev.Scale(factor)
	h.callback(cursor.Img, cursor.Width, cursor.Height, cursor.Hotx, cursor.Hoty)
}

func (h *CursorHandle) Start() chan struct{} {
	go start(h)
	quit := make(chan struct{})
	return quit
}
