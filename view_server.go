package gostream

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"image"
	"net/http"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/edaniels/golog"
	"github.com/edaniels/gostream/pkg/macos"
	"github.com/evangwt/go-vncproxy"
	"github.com/gorilla/mux"
	"golang.org/x/net/websocket"
)

// A ViewServer is a convenience helper for solely streaming a series
// Views. Views can be added over time for future new connections.
type ViewServer interface {
	// AddView adds the given view for new connections to see.
	AddView(view View)
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

func (rvs *viewServer) AddView(view View) {
	rvs.views = append(rvs.views, view)
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

	_, thisFilePath, _, _ := runtime.Caller(0)
	thisDirPath, err := filepath.Abs(filepath.Dir(thisFilePath))
	if err != nil {
		return fmt.Errorf("error locating current file: %w", err)
	}
	t, err := template.New("foo").Funcs(template.FuncMap{
		"jsSafe": func(js string) template.JS {
			return template.JS(js)
		},
		"htmlSafe": func(html string) template.HTML {
			return template.HTML(html)
		},
	}).ParseGlob(fmt.Sprintf("%s/assets/*.html.tpl", thisDirPath))
	if err != nil {
		return fmt.Errorf("error parsing templates: %w", err)
	}
	tmpl := t.Lookup("vnc.html.tpl")

	mux := mux.NewRouter() //http.NewServeMux()
	httpServer.Handler = mux

	staticDirectory := "./assets/static"
	staticPaths := map[string]string{
		"app":    filepath.Join(staticDirectory, "app"),
		"core":   filepath.Join(staticDirectory, "core"),
		"vendor": filepath.Join(staticDirectory, "vendor"),
	}
	for pathName, pathValue := range staticPaths {
		pathPrefix := "/" + pathName + "/"
		srv := http.FileServer(http.Dir(pathValue))
		mux.PathPrefix(pathPrefix).Handler(http.StripPrefix(pathPrefix, srv))
	}

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		bv := rvs.views[0]
		servers := bv.SinglePageHTML()

		if err := tmpl.Execute(w, servers); err != nil {
			rvs.logger.Error(err)
			return
		}
	})
	for _, view := range rvs.views {
		handler := view.Handler()
		fmt.Println("handler.Name", handler.Name)
		mux.Handle("/"+handler.Name, handler.Func)
	}

	vncProxy := vncproxy.New(&vncproxy.Config{
		LogLevel: vncproxy.DebugLevel,
		TokenHandler: func(r *http.Request) (addr string, err error) {
			addr = "192.168.55.1:5900"
			return
		},
	})
	proxy := websocket.Handler(vncProxy.ServeWS)
	mux.Handle("/vnc_ws", proxy)

	handle := macos.NewCursorHandle()
	handle.Start(func(img image.Image, width int, height int, hotx int, hoty int) {
		for _, view := range rvs.views {
			view.SendCursorToAll(img, width, height, hotx, hoty)
		}
		fmt.Println("detected new cursor", len(img.(*image.RGBA).Pix))
	})

	rvs.backgroundProcessing.Add(1)
	go func() {
		defer rvs.backgroundProcessing.Done()
		rvs.logger.Infow("listening", "url", fmt.Sprintf("http://0.0.0.0:%d", rvs.port), "port", rvs.port)
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
