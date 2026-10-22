//go:build !windows

package platform

func ShowFatal(title, msg string) bool {
	return false
}
