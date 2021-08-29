package worker

/*
#cgo CFLAGS: -I${SRCDIR}/bridge -x objective-c
#cgo LDFLAGS: -framework Foundation -framework AppKit
#import "library.m"
*/
import "C"

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"image"
	"io"
	"os"
	"sync"
	"time"
	"unsafe"

	"github.com/trevor403/gostream/pkg/common"
	"github.com/trevor403/gostream/pkg/ipcmsg"
	"golang.org/x/image/tiff"
)

const ipcmsgFd = 3
const maxMemBytes = 75 * 1024 * 1024

type Callback func(cursor common.CursorImage)

// Start the worker loop
func Start(callback Callback) {
	runner := func(data []byte) {
		cb := CursorBuffer{}

		r := bytes.NewReader(data)
		dec := gob.NewDecoder(r)
		err := dec.Decode(&cb)
		if err != nil {
			fmt.Println("DEC Error", err)
		}

		img := image.NewRGBA(image.Rect(0, 0, cb.Width, cb.Height))
		img.Pix = cb.Pix
		cursor := common.CursorImage{img, cb.Width, cb.Height, cb.Hotx, cb.Hoty}
		callback(cursor)
	}
	go run(runner)
}

func fork_main() {
	fmt.Println("child started")

	ppid := os.Getppid()
	r, w := ipcmsg.Channel(ppid, ipcmsgFd)
	_ = r

	h := NewHandle()
	ticker := time.NewTicker(33 * time.Millisecond)
	for _ = range ticker.C {
		cursor := h.getCursor()
		if h.compare(cursor.Img) != 0 {
			pix := cursor.Img.(*image.RGBA).Pix
			cb := CursorBuffer{pix, cursor.Width, cursor.Height, cursor.Hotx, cursor.Hoty}

			b := bytes.Buffer{}
			enc := gob.NewEncoder(&b)
			err := enc.Encode(cb)
			if err != nil {
				fmt.Println("ENC Error", err)
			}
			w <- ipcmsg.Message(42, b.Bytes())
		}
		h.prev = cursor
	}

	// heartbeat:
	// 	for {
	// 		select {
	// 		case <-time.After(15 * time.Second):
	// 			break heartbeat
	// 		case msg := <-r:
	// 			if msg.Hdr.Type != uint32(msg.Data[0]) {
	// 				fmt.Println("Error with heartbeat")
	// 				break heartbeat
	// 			}
	// 		}
	// 	}

	// 	fmt.Println("missing heartbeat - exiting...")
}

type Handle struct {
	mux sync.Mutex

	buf  []byte
	prev common.CursorImage
}

func NewHandle() *Handle {
	h := &Handle{}

	bufferSize := 4 * 1024 * 1024
	h.buf = make([]byte, bufferSize)

	return h
}

type CursorBuffer struct {
	Pix    []byte
	Width  int
	Height int
	Hotx   int
	Hoty   int
}

func (h *Handle) getCursor() common.CursorImage {
	h.mux.Lock()
	cbuf := (*C.char)(unsafe.Pointer(&h.buf[0]))

	metadata := C.cursor_metadata{}

	clen := C.readCursor(cbuf, &metadata)
	length := int64(clen)
	r := bytes.NewReader(h.buf)
	lr := io.LimitReader(r, length)

	img, err := tiff.Decode(lr)
	if err != nil {
		panic(err)
	}

	h.mux.Unlock()

	out := common.CursorImage{}
	out.Img = img
	out.Height = int(metadata.height)
	out.Width = int(metadata.width)
	out.Hotx = int(metadata.hotx)
	out.Hoty = int(metadata.hoty)

	return out
}

func (h *Handle) compare(img image.Image) int64 {
	raw1, ok := h.prev.Img.(*image.RGBA)
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
