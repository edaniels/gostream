// +build darwin

package platform

/*
#cgo CFLAGS: -I${SRCDIR}/bridge -x objective-c
#cgo LDFLAGS: -framework Foundation -framework AppKit
#import "library.m"
*/
import "C"
import (
	"bytes"
	"image"
	"io"
	"math"
	"sync"
	"time"
	"unsafe"

	"github.com/nfnt/resize"
	"golang.org/x/image/tiff"
)

type CursorHandle struct {
	mux      sync.Mutex
	callback UpdateCallback
	factor   float32
	buf      []byte
	prev     cursorImage
}

type cursorImage struct {
	img    image.Image
	width  int
	height int
	hotx   int
	hoty   int
}

func (c cursorImage) Scale(factor float32) cursorImage {
	out := cursorImage{}
	out.height = int(math.Round(float64(factor) * float64(c.height)))
	out.width = int(math.Round(float64(factor) * float64(c.width)))
	out.hotx = int(math.Round(float64(factor) * float64(c.hotx)))
	out.hoty = int(math.Round(float64(factor) * float64(c.hoty)))
	out.img = resize.Resize(uint(out.width), uint(out.height), c.img, resize.Lanczos3)
	return out
}

func NewCursorHandle() *CursorHandle {
	h := CursorHandle{}

	h.factor = 1.0

	bufferSize := 4 * 1024 * 1024
	h.buf = make([]byte, bufferSize)

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
	cursor := h.prev.Scale(factor)
	h.callback(cursor.img, cursor.width, cursor.height, cursor.hotx, cursor.hoty)
}

func (h *CursorHandle) Start() chan struct{} {
	ticker := time.NewTicker(33 * time.Millisecond)
	quit := make(chan struct{})
	go func() {
		for {
			select {
			case <-ticker.C:
				cursor := h.getCursor()
				if h.compare(cursor.img) != 0 {
					if h.callback != nil {
						scaled := cursor.Scale(h.factor)
						h.callback(scaled.img, scaled.width, scaled.height, scaled.hotx, scaled.hoty)
					}
					h.prev = cursor
				}
			case <-quit:
				ticker.Stop()
				return
			}
		}
	}()
	return quit
}

func (h *CursorHandle) getCursor() cursorImage {
	h.mux.Lock()
	cbuf := (*C.char)(unsafe.Pointer(&h.buf[0]))

	metadata := C.cursor_metadata{}

	clen := C.readCursor(cbuf, &metadata)
	length := int64(clen)
	r := bytes.NewReader(h.buf)
	lr := io.LimitReader(r, length)

	img, err := tiff.Decode(lr)
	// if err != nil {
	// 	panic(err)
	// }
	// HACK
	_ = err

	h.mux.Unlock()

	out := cursorImage{}
	out.img = img
	out.height = int(metadata.height)
	out.width = int(metadata.width)
	out.hotx = int(metadata.hotx)
	out.hoty = int(metadata.hoty)

	return out
}

func (h *CursorHandle) compare(img image.Image) int64 {
	raw1, ok := h.prev.img.(*image.RGBA)
	if !ok {
		return -1
	}

	raw2, ok := img.(*image.RGBA)
	if !ok {
		return -2
	}

	if raw1.Bounds() != raw2.Bounds() {
		return -3
	}

	if bytes.Equal(raw1.Pix, raw2.Pix) {
		return 0
	}

	return 1
}
