package platform

import "image"

type UpdateCallback func(img image.Image, width int, height int, hotx int, hoty int)
