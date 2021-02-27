package gostream

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/draw"
	"io/ioutil"

	"github.com/edaniels/golog"
)

type ImageSource interface {
	Next(ctx context.Context) (image.Image, error)
	Close() error
}

type NoCtxImageSource interface {
	Next() (image.Image, error)
	Close() error
}

type ImageSourceFunc func(ctx context.Context) (image.Image, error)

func (isf ImageSourceFunc) Next(ctx context.Context) (image.Image, error) {
	return isf(ctx)
}

func (isf ImageSourceFunc) Close() error {
	return nil
}

func streamSource(ctx context.Context, once func(), is ImageSource, name string, remoteView RemoteView) {
	if once != nil {
		once()
	}
	stream := remoteView.ReserveStream(name)
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
		frame, err := is.Next(ctx)
		if err != nil {
			golog.Global.Debugw("error getting frame", "error", err)
			continue
		}
		stream.InputFrames() <- frame
	}
}

func StreamSource(ctx context.Context, is ImageSource, remoteView RemoteView) {
	StreamSourceOnce(ctx, nil, is, remoteView)
}

func StreamSourceOnce(ctx context.Context, once func(), is ImageSource, remoteView RemoteView) {
	streamSource(ctx, once, is, "", remoteView)
}

//nolint:interfacer
func StreamFunc(ctx context.Context, isf ImageSourceFunc, remoteView RemoteView) {
	StreamSourceOnce(ctx, nil, isf, remoteView)
}

//nolint:interfacer
func StreamFuncOnce(ctx context.Context, once func(), isf ImageSourceFunc, remoteView RemoteView) {
	streamSource(ctx, once, isf, "", remoteView)
}

func StreamNamedSource(ctx context.Context, is ImageSource, name string, remoteView RemoteView) {
	StreamNamedSourceOnce(ctx, nil, is, name, remoteView)
}

func StreamNamedSourceOnce(ctx context.Context, once func(), is ImageSource, name string, remoteView RemoteView) {
	streamSource(ctx, once, is, name, remoteView)
}

//nolint:interfacer
func StreamNamedFunc(ctx context.Context, isf ImageSourceFunc, name string, remoteView RemoteView) {
	StreamNamedFuncOnce(ctx, nil, isf, name, remoteView)
}

//nolint:interfacer
func StreamNamedFuncOnce(ctx context.Context, once func(), isf ImageSourceFunc, name string, remoteView RemoteView) {
	streamSource(ctx, once, isf, name, remoteView)
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

// to RGBA, may be lossy
func CloneImage(src image.Image) image.Image {
	bounds := src.Bounds()
	dst := image.NewRGBA(bounds)
	draw.Draw(dst, bounds, src, bounds.Min, draw.Src)
	return dst
}
