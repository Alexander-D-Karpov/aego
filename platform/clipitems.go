package platform

type ClipItem struct {
	Mime string
	Data []byte
}

type ClipboardItems interface {
	SetClipboardItems(items []ClipItem) bool
	ClipboardMimes() []string
}
