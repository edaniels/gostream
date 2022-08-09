package media

import (
	"github.com/pion/mediadevices/pkg/driver"
	"github.com/pion/mediadevices/pkg/io/audio"
	"github.com/pion/mediadevices/pkg/wave"
)

// NewAudioReadCloser instantiates a new audio read closer and references the given
// driver.
func NewAudioReadCloser(d driver.Driver, r audio.Reader) ReadCloser[wave.Audio] {
	return newReadCloser[wave.Audio](d, r)
}
