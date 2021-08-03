package platform

/*
#cgo CFLAGS: -I${SRCDIR}/bridge -x objective-c
#cgo LDFLAGS: -L${SRCDIR}/bridge -framework Foundation -framework AppKit -mmacosx-version-min=10.15 -lFBRetainCycleDetector
#import "library.h"
*/
import "C"
import (
	"bytes"
	"image"
	"io"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/image/tiff"
)

var hookInstalled = false

type CursorHandle struct {
	mux  sync.Mutex
	buf  []byte
	prev cursorImage
}

type cursorImage struct {
	img    image.Image
	width  int
	height int
	hotx   int
	hoty   int
}

type UpdateCallback func(img image.Image, width int, height int, hotx int, hoty int)

func NewCursorHandle() *CursorHandle {
	h := CursorHandle{}
	h.installHook()

	bufferSize := 4 * 1024 * 1024
	h.buf = make([]byte, bufferSize)

	return &h
}

func (h *CursorHandle) Start(callback UpdateCallback) chan struct{} {
	ticker := time.NewTicker(33 * time.Millisecond)
	quit := make(chan struct{})
	go func() {
		for {
			select {
			case <-ticker.C:
				cursor := h.getCursor()
				if h.compare(cursor.img) != 0 {
					callback(cursor.img, cursor.width, cursor.height, cursor.hotx, cursor.hoty)
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

func (h *CursorHandle) installHook() {
	if !hookInstalled {
		C.installHook()
		hookInstalled = true
	}
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
	// if h.prev == nil {
	// 	return -1
	// }

	// fmt.Println(img.(*image.NRGBA))

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
	// accumError := int64(0)

	// for i := 0; i < len(raw1.Pix); i++ {
	// 	accumError += int64(sqDiffUInt8(raw1.Pix[i], raw2.Pix[i]))
	// }

	// return int64(math.Sqrt(float64(accumError)))
}

// func sqDiffUInt8(x, y uint8) uint64 {
// 	d := uint64(x) - uint64(y)
// 	return d * d
// }
