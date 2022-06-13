package gostream

import (
	"context"
	"errors"
	"image"
	"math"
	"testing"
	"time"

	"github.com/pion/webrtc/v3"
)

var errImageRetrieval = errors.New("image retrieval failed")

type mockErrorImageSource struct {
	called   int
	maxCalls int
}

func (imageSource *mockErrorImageSource) Next(ctx context.Context) (image.Image, func(), error) {
	if imageSource.called < imageSource.maxCalls {
		imageSource.called += 1
	}
	return nil, nil, errImageRetrieval
}

func (imageSource *mockErrorImageSource) Called() int {
	return imageSource.called
}

func (imageSource *mockErrorImageSource) MaxCalls() int {
	return imageSource.maxCalls
}

type mockStream struct {
	name               string
	streamingReadyFunc func() <-chan struct{}
	inputFramesFunc    func() chan<- FrameReleasePair
}

func (mS *mockStream) StreamingReady() <-chan struct{} {
	return mS.streamingReadyFunc()
}

func (mS *mockStream) InputFrames() chan<- FrameReleasePair {
	return mS.inputFramesFunc()
}

func (mS *mockStream) Name() string {
	return mS.name
}

func (mS *mockStream) Start() {
}

func (mS *mockStream) Stop() {
}

func (mS *mockStream) TrackLocal() webrtc.TrackLocal {
	return nil
}

func TestStreamSourceErrorBackoff(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	backoffOpts := &BackoffTuningOptions{
		ExpBase:          6.0,
		Offset:           0,
		MaxSleepMilliSec: math.Pow10(6) * 2, // two seconds
		MaxSleepAttempts: 3,
	}
	imgSrc := &mockErrorImageSource{maxCalls: 5}
	totalExpectedSleep := 0
	for i := 1; i < imgSrc.MaxCalls(); i++ {
		totalExpectedSleep += backoffOpts.GetSleepTimeFromErrorCount(i)
	}
	str := &mockStream{}
	readyChan := make(chan struct{})
	inputChan := make(chan FrameReleasePair)
	str.streamingReadyFunc = func() <-chan struct{} {
		return readyChan
	}
	str.inputFramesFunc = func() chan<- FrameReleasePair {
		return inputChan
	}
	go StreamSourceWithOptions(ctx, imgSrc, str, backoffOpts)
	readyChan <- struct{}{}
	time.Sleep(time.Duration(totalExpectedSleep) + 1000)
	cancel()
	timesCalled := imgSrc.Called()
	expectedCalls := imgSrc.MaxCalls()
	if imgSrc.Called() != imgSrc.MaxCalls() {
		t.Errorf("expected %d sleep calls but got %d", timesCalled, expectedCalls)
	}
}
