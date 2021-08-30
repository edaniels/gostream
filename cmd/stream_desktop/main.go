package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"runtime"
	"runtime/pprof"
	"syscall"

	"github.com/trevor403/gostream"
	"github.com/trevor403/gostream/codec/x264"
	"github.com/trevor403/gostream/media"
)

func main() {
	cpuprofile := flag.String("cpuprofile", "", "write cpu profile to `file`")
	memprofile := flag.String("memprofile", "", "write mem profile to `file`")

	port := flag.Int("port", 5555, "port to run server on")
	camera := flag.Bool("camera", false, "use camera")
	flag.Parse()

	if *cpuprofile != "" {
		f, err := os.Create(*cpuprofile)
		if err != nil {
			log.Fatal("could not create CPU profile: ", err)
		}
		defer f.Close() // error handling omitted for example
		if err := pprof.StartCPUProfile(f); err != nil {
			log.Fatal("could not start CPU profile: ", err)
		}
		defer pprof.StopCPUProfile()
	}

	if *memprofile != "" {
		f, err := os.Create(*memprofile)
		if err != nil {
			log.Fatal("could not create memory profile: ", err)
		}
		defer f.Close() // error handling omitted for example
		runtime.GC()    // get up-to-date statistics
		if err := pprof.WriteHeapProfile(f); err != nil {
			log.Fatal("could not write memory profile: ", err)
		}
	}

	var videoReader media.VideoReadCloser
	var err error
	if *camera {
		videoReader, err = media.GetAnyVideoReader(media.DefaultConstraints)
	} else {
		videoReader, err = media.GetAnyScreenReader(media.DefaultConstraints)
	}
	if err != nil {
		gostream.Logger.Fatal(err)
	}

	defer func() {
		if err := videoReader.Close(); err != nil {
			gostream.Logger.Error(err)
		}
	}()

	config := x264.DefaultViewConfig
	// config := vpx.DefaultViewConfig
	config.TargetFrameRate = 30
	view, err := gostream.NewView(config)
	if err != nil {
		gostream.Logger.Fatal(err)
	}

	server := gostream.NewViewServer(*port, view, gostream.Logger)
	if err := server.Start(); err != nil {
		gostream.Logger.Fatal(err)
	}

	cancelCtx, cancelFunc := context.WithCancel(context.Background())
	c := make(chan os.Signal, 2)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		cancelFunc()
	}()

	gostream.StreamNamedSource(cancelCtx, videoReader, "screen", view)

	if err := server.Stop(context.Background()); err != nil {
		gostream.Logger.Error(err)
	}
}
