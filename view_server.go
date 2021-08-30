package gostream

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"image"
	"io"
	"io/fs"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/edaniels/golog"
	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/trevor403/gostream/pkg/input/direct"
	"github.com/trevor403/gostream/pkg/platform"
)

// A ViewServer is a convenience helper for solely streaming a series
// Views. Views can be added over time for future new connections.
type ViewServer interface {
	// Start starts the server and waits for new connections.
	Start() error
	// Stop stops the server and stops the underlying views.
	Stop(ctx context.Context) error
}

type viewServer struct {
	port                 int
	views                []View
	httpServer           *http.Server
	started              bool
	logger               golog.Logger
	backgroundProcessing sync.WaitGroup
}

// NewViewServer returns a server that will run on the given port and initially starts
// with the given view.
func NewViewServer(port int, view View, logger golog.Logger) ViewServer {
	return &viewServer{port: port, views: []View{view}, logger: logger}
}

// ErrServerAlreadyStarted happens when the server has already been started.
var ErrServerAlreadyStarted = errors.New("already started")

func (rvs *viewServer) Start() error {
	if rvs.started {
		return ErrServerAlreadyStarted
	}
	rvs.started = true
	httpServer := &http.Server{
		Addr:           fmt.Sprintf(":%d", rvs.port),
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}
	rvs.httpServer = httpServer

	mux := mux.NewRouter()
	// http.NewServeMux()
	httpServer.Handler = mux

	staticDirectory := "assets/static"
	staticPaths := map[string]embed.FS{
		"core": coreFS,
	}
	for pathName, pathFS := range staticPaths {
		pathPrefix := "/" + pathName + "/"

		fs, err := fs.Sub(pathFS, staticDirectory)
		fmt.Println(err)
		srv := http.FileServer(http.FS(fs))
		mux.PathPrefix(pathPrefix).Handler(srv)
	}

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path[1:]
		if path == "" {
			path = "index.html"
		}
		s := strings.NewReader(mainHTML)
		http.ServeContent(w, r, "index.html", time.Now(), s)
	})

	mux.HandleFunc("/servers", func(w http.ResponseWriter, r *http.Request) {
		bv := rvs.views[0]
		servers := bv.SinglePageHTML()
		io.WriteString(w, servers)
	})
	for _, view := range rvs.views {
		handler := view.Handler()
		fmt.Println("handler.Name", handler.Name)
		mux.Handle("/"+handler.Name, handler.Func)
	}

	handle := platform.NewCursorHandle()
	handle.SetCallback(func(img image.Image, width int, height int, hotx int, hoty int) {
		for _, view := range rvs.views {
			view.SendCursorToAll(img, width, height, hotx, hoty)
		}
		fmt.Println("detected new cursor", len(img.(*image.RGBA).Pix))
	})
	handle.Start()

	for _, view := range rvs.views {
		view.SetOnSizeHandler(func(ctx context.Context, factor float32, responder ClientResponder) {
			if factor > 5 {
				factor = 1
			}
			Logger.Debugw("scaled", "factor", factor)
			handle.UpdateScale(factor)
		})

		view.SetOnDataHandler(func(ctx context.Context, data []byte, responder ClientResponder) {
			direct.Handle(data)
		})
	}

	mux.Use(func(next http.Handler) http.Handler { return handlers.LoggingHandler(os.Stdout, next) })

	rvs.backgroundProcessing.Add(1)
	go func() {
		defer rvs.backgroundProcessing.Done()
		addr, err := LocalIP()
		if err != nil {
			rvs.logger.Errorw("error getting local ip", "error", err)
			return
		}
		rvs.logger.Infow("listening", "url", fmt.Sprintf("http://%v:%d", addr, rvs.port), "port", rvs.port)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			rvs.logger.Errorw("error listening and serving", "error", err)
		}
	}()
	return nil
}

func (rvs *viewServer) Stop(ctx context.Context) error {
	for _, view := range rvs.views {
		view.Stop()
	}
	err := rvs.httpServer.Shutdown(ctx)
	rvs.backgroundProcessing.Wait()
	return err
}
