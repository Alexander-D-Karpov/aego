package shell

import (
	"strconv"
	"strings"

	"aego/core/ids"
	"aego/editor/core/context"
)

// openAgentLink follows a link of the Agent panel: doc:<id> or ref:<id>, node:<file>#<id>, asset:<path>, file:<path>:<line> (11b5a).
func (a *App) openAgentLink(link string) {
	kind, rest, _ := strings.Cut(link, ":")
	switch kind {
	case "doc", "ref":
		a.ctx.Open(context.DocsAt(rest, ""))
	case "node":
		file, hex, _ := strings.Cut(rest, "#")
		if id, err := ids.ParseNodeID(hex); err == nil {
			a.ctx.Open(context.AtNode(file, id))
		}
	case "asset":
		a.ctx.Open(context.AssetAt(rest))
	case "file":
		p, line := rest, 0
		if i := strings.LastIndexByte(rest, ':'); i > 0 {
			if n, err := strconv.Atoi(rest[i+1:]); err == nil {
				p, line = rest[:i], n
			}
		}
		a.ctx.Open(context.At(p, line, 0))
	}
}
