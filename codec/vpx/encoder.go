package vpx

import (
	"errors"
	"fmt"
	"image"
	"image/draw"
	"runtime"
	"time"
	"unsafe"

	"github.com/edaniels/golog"

	govpx "github.com/xlab/libvpx-go/vpx"
)

/*
#cgo pkg-config: vpx
#include <vpx/vpx_encoder.h>

typedef struct GoBytes {
  void *bs;
  int size;
} GoBytesType;

GoBytesType get_frame_buffer(vpx_codec_cx_pkt_t *pkt) {
	// iter has set to NULL when after add new image
	GoBytesType bytes = {NULL, 0};
	if (pkt->kind == VPX_CODEC_CX_FRAME_PKT) {
		bytes.bs = pkt->data.frame.buf;
		bytes.size = pkt->data.frame.sz;
	} else {
		bytes.size = 999;
	}
  return bytes;
}

#include <string.h>
int vpx_img_plane_width(const vpx_image_t *img, int plane) {
  if (plane > 0 && img->x_chroma_shift > 0)
    return (img->d_w + 1) >> img->x_chroma_shift;
  else
    return img->d_w;
}
int vpx_img_plane_height(const vpx_image_t *img, int plane) {
  if (plane > 0 && img->y_chroma_shift > 0)
    return (img->d_h + 1) >> img->y_chroma_shift;
  else
    return img->d_h;
}
int vpx_img_read(vpx_image_t *img, void *bs) {
  int plane;
  for (plane = 0; plane < 3; ++plane) {
    unsigned char *buf = img->planes[plane];
    const int stride = img->stride[plane];
    const int w = vpx_img_plane_width(img, plane) *
                  ((img->fmt & VPX_IMG_FMT_HIGHBITDEPTH) ? 2 : 1);
    const int h = vpx_img_plane_height(img, plane);
    int y;
    for (y = 0; y < h; ++y) {
      memcpy(buf, bs, w);
      // if (fread(buf, 1, w, file) != (size_t)w) return 0;
      buf += stride;
      bs += w;
    }
  }
  return 1;
}
*/
import "C"

type encoder struct {
	ctx              *govpx.CodecCtx
	iface            *govpx.CodecIface
	allocImg         *govpx.Image
	iter             govpx.CodecIter
	keyFrameInterval int
	frameCount       int
	debug            bool
	logger           golog.Logger
}

type VCodec string

const (
	CodecVP8 VCodec = "V_VP8"
	CodecVP9 VCodec = "V_VP9"
)

func NewEncoder(codec VCodec, width, height int, debug bool, logger golog.Logger) (*encoder, error) {
	enc := &encoder{ctx: govpx.NewCodecCtx(), debug: debug, logger: logger}
	switch codec {
	case CodecVP8:
		enc.iface = govpx.EncoderIfaceVP8()
	case CodecVP9:
		enc.iface = govpx.EncoderIfaceVP9()
	default:
		return nil, fmt.Errorf("[WARN] unsupported VPX codec: %s", codec)
	}

	var cfg govpx.CodecEncCfg
	enc.keyFrameInterval = 1 // MAYBE 5
	err := govpx.Error(govpx.CodecEncConfigDefault(enc.iface, &cfg, 0))
	if err != nil {
		panic(err)
	}
	cfg.Deref()
	cfg.GW = uint32(width)
	cfg.GH = uint32(height)
	cfg.GTimebase = govpx.Rational{
		Num: 1,
		Den: 33,
	}
	cfg.RcTargetBitrate = 200000
	cfg.GErrorResilient = 1
	cfg.Free() // free so we get a new one? idk

	abiVersion := govpx.EncoderABIVersion
	if runtime.GOOS != "darwin" {
		abiVersion++
	}
	err = govpx.Error(govpx.CodecEncInitVer(enc.ctx, enc.iface, &cfg, 0, int32(abiVersion)))
	if err != nil {
		logger.Warn(err)
		return enc, nil
	}

	var cImg govpx.Image
	allocImg := govpx.ImageAlloc(&cImg, govpx.ImageFormatI420, uint32(width), uint32(height), 0)
	if allocImg == nil {
		return nil, errors.New("failed to allocate image")
	}
	allocImg.Deref()

	enc.allocImg = allocImg

	return enc, nil
}

func (v *encoder) Encode(img image.Image) ([]byte, error) {
	// would want to use the iter in encoder but right now using it
	// causes "cgo argument has Go pointer to Go pointer"
	var iter govpx.CodecIter
	iterate := func() ([]byte, error) {
		pkt := govpx.CodecGetCxData(v.ctx, &iter)
		// println("pkt", pkt)
		for pkt != nil {
			pkt.Deref()
			data := func() []byte {
				defer pkt.Free()
				if pkt.Kind == govpx.CodecCxFramePkt {
					now := time.Now()
					goBytes := C.get_frame_buffer((*C.vpx_codec_cx_pkt_t)(unsafe.Pointer(pkt.Ref())))
					bs := C.GoBytes(goBytes.bs, goBytes.size)
					if v.debug {
						v.logger.Debugw("got frame", "elapsed", time.Since(now))
					}
					return bs
				}
				return nil
			}()
			if data != nil {
				return data, nil
			}
			pkt = govpx.CodecGetCxData(v.ctx, &iter)
		}
		return nil, nil
	}
	if v.iter != nil {
		return iterate()
	}

	bounds := img.Bounds()
	imRGBA := image.NewRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	draw.Draw(imRGBA, imRGBA.Bounds(), img, bounds.Min, draw.Src)

	yuvImage := rgbaToYuv(imRGBA)
	C.vpx_img_read((*C.vpx_image_t)(unsafe.Pointer(v.allocImg)), unsafe.Pointer(&yuvImage[0]))

	var flags govpx.EncFrameFlags
	if v.keyFrameInterval > 0 && v.frameCount%v.keyFrameInterval == 0 {
		flags |= 1 // C.VPX_EFLAG_FORCE_KF
	}

	// println("encoding frame", v.frameCount)
	now := time.Now()
	if err := govpx.Error(govpx.CodecEncode(v.ctx, v.allocImg, govpx.CodecPts(v.frameCount), 1, flags, 30)); err != nil {
		return nil, errors.New(govpx.CodecErrorDetail(v.ctx))
	}
	if v.debug {
		v.logger.Debugw("encoded frame", "elapsed", time.Since(now))
	}
	v.frameCount++

	v.iter = nil
	return iterate()
}
