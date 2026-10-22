package platform

import "image"

type Iconer interface {
	SetIcon([]image.Image)
}

var appIcon []image.Image

func SetAppIcon(imgs ...image.Image) {
	appIcon = imgs
}

func AppIcon() []image.Image {
	return appIcon
}
