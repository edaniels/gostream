package vpx

/*
// https://stackoverflow.com/questions/9465815/rgb-to-yuv420-algorithm-efficiency
void rgba2yuv(void *destination, void *source, int width, int height, int stride) {
	const int image_size = width * height;
	unsigned char *rgba = source;
  unsigned char *dst_y = destination;
  unsigned char *dst_u = destination + image_size;
  unsigned char *dst_v = destination + image_size + image_size/4;
	// Y plane
	for( int y=0; y<height; ++y ) {
    for( int x=0; x<width; ++x ) {
      const int i = y*(width+stride) + x;
			*dst_y++ = ( ( 66*rgba[4*i] + 129*rgba[4*i+1] + 25*rgba[4*i+2] ) >> 8 ) + 16;
		}
  }
  // U plane
  for( int y=0; y<height; y+=2 ) {
    for( int x=0; x<width; x+=2 ) {
      const int i = y*(width+stride) + x;
			*dst_u++ = ( ( -38*rgba[4*i] + -74*rgba[4*i+1] + 112*rgba[4*i+2] ) >> 8 ) + 128;
		}
  }
  // V plane
  for( int y=0; y<height; y+=2 ) {
    for( int x=0; x<width; x+=2 ) {
      const int i = y*(width+stride) + x;
			*dst_v++ = ( ( 112*rgba[4*i] + -94*rgba[4*i+1] + -18*rgba[4*i+2] ) >> 8 ) + 128;
		}
  }
}
*/
import "C"
import (
	"fmt"

	"github.com/edaniels/golog"
	"github.com/edaniels/gostream"
)

var DefaultRemoteViewConfig = gostream.PartialDefaultRemoteViewConfig

func init() {
	DefaultRemoteViewConfig.EncoderFactory = NewEncoderFactory(CodecVP8, false)
}

func NewEncoderFactory(codec VCodec, debug bool) gostream.EncoderFactory {
	return &factory{codec, debug}
}

type factory struct {
	codec VCodec
	debug bool
}

func (f *factory) New(width, height int, logger golog.Logger) (gostream.Encoder, error) {
	return NewEncoder(f.codec, width, height, f.debug, logger)
}

func (f *factory) MIMEType() string {
	switch f.codec {
	case CodecVP8:
		return "video/vp8"
	case CodecVP9:
		return "video/vp9"
	default:
		panic(fmt.Errorf("unknown codec %q", f.codec))
	}
}
