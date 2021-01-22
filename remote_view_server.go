package gostream

import (
	"context"
	"fmt"
	"html/template"
	"net/http"
	"path/filepath"
	"runtime"
	"time"

	"github.com/edaniels/golog"
)

type RemoteViewServer interface {
	AddView(view RemoteView)
	Run()
	Stop(ctx context.Context) error
}

type remoteViewServer struct {
	port        int
	remoteViews []RemoteView
	httpServer  *http.Server
	running     bool
	logger      golog.Logger
}

func NewRemoteViewServer(port int, view RemoteView, logger golog.Logger) RemoteViewServer {
	return &remoteViewServer{port: port, remoteViews: []RemoteView{view}, logger: logger}
}

func (rvs *remoteViewServer) AddView(view RemoteView) {
	rvs.remoteViews = append(rvs.remoteViews, view)
}

func (rvs *remoteViewServer) Run() {
	if rvs.running {
		panic("already running")
	}
	rvs.running = true
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
		panic(err)
	}
	t, err := template.New("foo").Funcs(template.FuncMap{
		"jsSafe": func(js string) template.JS {
			return template.JS(js)
		},
		"htmlSafe": func(html string) template.HTML {
			return template.HTML(html)
		},
	}).ParseGlob(fmt.Sprintf("%s/*.html", thisDirPath))
	if err != nil {
		panic(err)
	}
	template := t.Lookup("remote_view_multi.html")

	mux := http.NewServeMux()
	httpServer.Handler = mux
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if len(rvs.remoteViews) == 1 {
			if _, err := w.Write([]byte(rvs.remoteViews[0].SinglePageHTML())); err != nil {
				rvs.logger.Error(err)
			}
			return
		}
		type Temp struct {
			RemoteViews []RemoteViewHTML
		}

		temp := Temp{}
		for _, remoteView := range rvs.remoteViews {
			htmlData := remoteView.HTML()
			temp.RemoteViews = append(temp.RemoteViews, RemoteViewHTML{
				htmlData.JavaScript,
				htmlData.Body,
			})
		}

		err := template.Execute(w, temp)
		if err != nil {
			rvs.logger.Errorw("couldn't execute web page", "error", err)
		}
	})
	for _, view := range rvs.remoteViews {
		handler := view.Handler()
		mux.Handle("/"+handler.Name, handler.Func)
	}

	go func() {
		rvs.logger.Infow("listening", "url", fmt.Sprintf("http://localhost:%d", rvs.port), "port", rvs.port)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			panic(err)
		}
	}()
}

func (rvs *remoteViewServer) Stop(ctx context.Context) error {
	for _, view := range rvs.remoteViews {
		view.Stop()
	}
	return rvs.httpServer.Shutdown(ctx)
}
