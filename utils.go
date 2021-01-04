package gostream

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"io/ioutil"
	"time"
)

func streamFunc(ctx context.Context, once func(), f func() image.Image, remoteView RemoteView, captureInternal time.Duration) {
	if once != nil {
		once()
	}
	select {
	case <-ctx.Done():
		return
	case <-remoteView.Ready():
	}
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		time.Sleep(captureInternal)
		remoteView.InputFrames() <- f()
	}
}

func StreamFunc(ctx context.Context, f func() image.Image, remoteView RemoteView, captureInternal time.Duration) {
	StreamFuncOnce(ctx, nil, f, remoteView, captureInternal)
}

func StreamFuncOnce(ctx context.Context, once func(), f func() image.Image, remoteView RemoteView, captureInternal time.Duration) {
	streamFunc(ctx, once, f, remoteView, captureInternal)
}

// Allows compressing offer/answer to bypass terminal input limits.
const compress = false

// Encode encodes the input in base64
// It can optionally zip the input before encoding
func Encode(obj interface{}) string {
	b, err := json.Marshal(obj)
	if err != nil {
		panic(err)
	}

	if compress {
		b = zip(b)
	}

	return base64.StdEncoding.EncodeToString(b)
}

// Decode decodes the input from base64
// It can optionally unzip the input after decoding
func Decode(in string, obj interface{}) {
	b, err := base64.StdEncoding.DecodeString(in)
	if err != nil {
		panic(err)
	}

	if compress {
		b = unzip(b)
	}

	err = json.Unmarshal(b, obj)
	if err != nil {
		panic(err)
	}
}

func zip(in []byte) []byte {
	var b bytes.Buffer
	gz := gzip.NewWriter(&b)
	_, err := gz.Write(in)
	if err != nil {
		panic(err)
	}
	err = gz.Flush()
	if err != nil {
		panic(err)
	}
	err = gz.Close()
	if err != nil {
		panic(err)
	}
	return b.Bytes()
}

func unzip(in []byte) []byte {
	var b bytes.Buffer
	_, err := b.Write(in)
	if err != nil {
		panic(err)
	}
	r, err := gzip.NewReader(&b)
	if err != nil {
		panic(err)
	}
	res, err := ioutil.ReadAll(r)
	if err != nil {
		panic(err)
	}
	return res
}
