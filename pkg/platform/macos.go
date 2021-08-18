// +build darwin

package platform

import (
	"github.com/trevor403/gostream/pkg/common"
	"github.com/trevor403/gostream/pkg/worker"
)

func start(h *CursorHandle) {
	worker.Start(func(cursor common.CursorImage) {
		if h.callback != nil {
			scaled := cursor.Scale(h.factor)
			h.callback(scaled.Img, scaled.Width, scaled.Height, scaled.Hotx, scaled.Hoty)
		}
		h.prev = cursor
	})
}
