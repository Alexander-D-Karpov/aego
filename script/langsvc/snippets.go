package langsvc

import "aego/script/workspace"

var scriptSnippets = []Item{
	{Label: "Ready", Detail: "lifecycle method", Kind: ItemSnippet, Insert: "func Ready() {\n\t\t${1}\n\t}"},
	{Label: "Update", Detail: "lifecycle method", Kind: ItemSnippet, Insert: "func Update(dt float) {\n\t\t${1}\n\t}"},
	{Label: "FixedUpdate", Detail: "lifecycle method", Kind: ItemSnippet, Insert: "func FixedUpdate(dt float) {\n\t\t${1}\n\t}"},
	{Label: "Reloaded", Detail: "lifecycle method", Kind: ItemSnippet, Insert: "func Reloaded() {\n\t\t${1}\n\t}"},
	{Label: "coroutine", Detail: "coroutine method", Kind: ItemSnippet, Insert: "func ${1:Name}() coroutine {\n\t\t${2}\n\t}"},
	{Label: "signal", Detail: "signal declaration", Kind: ItemSnippet, Insert: "signal ${1:Name}(${2})"},
	{Label: "export", Detail: "@export var field", Kind: ItemSnippet, Insert: "@export var ${1:Name} ${2:float}"},
	{Label: "pause", Detail: "@pause script attribute", Kind: ItemSnippet, Insert: "@pause(\"${1:always}\")"},
}

var topSnippets = []Item{
	{Label: "script", Detail: "script type with Update", Kind: ItemSnippet, Insert: "script $0 {\n\tfunc Update(dt float) {\n\t}\n}"},
	{Label: "enum", Detail: "enum declaration", Kind: ItemSnippet, Insert: "enum $0 {\n}"},
	{Label: "type", Detail: "struct declaration", Kind: ItemSnippet, Insert: "type $0 struct {\n}"},
}

func snippet(snap *workspace.Snapshot, it Item) Item {
	it.Doc = builtinText(snap, it.Label)
	return it
}
