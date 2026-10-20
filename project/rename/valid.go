package rename

import "strings"

func ValidName(s string) bool {
	return s != "" && s != "." && s != ".." && !strings.HasPrefix(s, ".") && !strings.ContainsAny(s, "/\\")
}
