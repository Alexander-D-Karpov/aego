package main

import (
	"fmt"
	"strings"

	"aego/app/assembly/builtin"
)

func main() {
	var dirs []string
	for _, d := range builtin.IconSources() {
		dirs = append(dirs, "../../"+d)
	}
	fmt.Print(strings.Join(dirs, ","))
}
