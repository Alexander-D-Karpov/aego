package langsvc

import (
	"aego/script/checker"
	"aego/script/lexer"
	"aego/script/resolver"
	"aego/script/source"
	"aego/script/token"
	"aego/script/workspace"
)

var builtinOrder = []string{
	"bool", "int", "float", "string", "Vec2", "Vec3", "Color", "Quat", "Rect",
	"Transform2D", "Transform3D", "NodeRef", "AssetRef", "ComponentRef",
	"Coroutine", "Connection", "ScriptRef",
}

var builtinDecl = map[string]string{
	"Vec2":         "type Vec2 struct { X, Y float }",
	"Vec3":         "type Vec3 struct { X, Y, Z float }",
	"Color":        "type Color struct { R, G, B, A float }",
	"Quat":         "type Quat struct { X, Y, Z, W float }",
	"Rect":         "type Rect struct { X, Y, W, H float }",
	"Transform2D":  "type Transform2D struct { Position Vec2; Rotation float; Scale Vec2 }",
	"Transform3D":  "type Transform3D struct { Position Vec3; Rotation Quat; Scale Vec3 }",
	"AssetRef":     "type AssetRef<Texture | Scene | Prefab | Mesh>",
	"ComponentRef": "type ComponentRef<T>",
	"Coroutine":    "type Coroutine",
	"Connection":   "type Connection",
	"ScriptRef":    "type ScriptRef<Script>",
}

var builtinDocs = map[string]string{
	"package":      "package names the package of this file. It must equal the directory name under scripts/; all files in one directory share it.",
	"import":       "import makes a script package or an engine module available by the last element of its path.\n\nimport (\n\t\"aego/input\"\n\t\"mygame/combat\"\n)",
	"script":       "script declares a stateful type that attaches to scene nodes. A node holds at most one instance of each script type.\nvar fields are runtime state; @export fields are saved in the scene and shown in the Inspector.\n\nscript Mover {\n\t@export var Speed float = 200\n\tfunc Update(dt float) {\n\t}\n}",
	"type":         "type declares a struct. Structs are values: assignment and passing copy them.\n\ntype Stats struct {\n\tHp    int\n\tArmor float\n}",
	"struct":       "struct lists the fields of a type declaration. Fields may be scalars, vectors, references, enums, slices or other structs.",
	"enum":         "enum declares named int constants used as Name.Member. A switch on an enum must cover every member or have a default.\n\nenum State {\n\tIdle\n\tRun\n}",
	"var":          "var declares a variable with an explicit type and an optional value. Inside a script it declares a field.\n\nvar hits []int\nvar speed float = 1.5",
	"const":        "const declares a compile-time constant of type bool, int, float, string or enum. Packages hold constants only, never mutable variables.\n\nconst MaxSpeed float = 600",
	"func":         "func declares a function. Package functions see no script state; functions inside a script are methods and can use node and the fields.\n\nfunc clamp01(x float) float {\n\treturn math.Clamp(x, 0, 1)\n}",
	"return":       "return ends the function, with a value when the function declares a result. Such a function must return on every path.",
	"if":           "if runs a block when the bool condition is true. No parentheses around the condition.\n\nif hp <= 0 {\n\tdie()\n} else if hp < 10 {\n\tflee()\n}",
	"else":         "else runs when the preceding if condition is false. It must stay on the line of the closing brace: } else {",
	"for":          "for is the only loop: for cond {}, for init; cond; post {} or for {} for an endless loop.\n\nfor i := 0; i < len(items); i++ {\n}",
	"switch":       "switch compares a value with each case and runs the first match. There is no fallthrough.\n\nswitch state {\ncase State.Idle:\ncase State.Run, State.Walk:\ndefault:\n}",
	"case":         "case lists one or more values for a switch branch.",
	"default":      "default runs when no case of the switch matches.",
	"break":        "break leaves the innermost for loop or switch.",
	"continue":     "continue jumps to the next iteration of the innermost for loop.",
	"true":         "true is the bool constant for yes.",
	"false":        "false is the bool constant for no.",
	"nil":          "nil is the empty reference. Only NodeRef, AssetRef and ComponentRef can be nil; reading through nil faults with AES6004.\n\nif Target != nil {\n}",
	"@export":      "@export saves the field in the scene and shows it in the Inspector.\nArguments: min, max, step (numbers only), unit, display, tip, category.\n\n@export(min=0, max=600, unit=\"px/s\")\nvar Speed float = 220",
	"@former":      "@former lists old names so scenes saved before a rename still load; the scene is rewritten with the new name on save.\n\n@former(\"Velocity\")\n@export var Speed float",
	"@hide":        "@hide keeps the script out of the Add Script menu. Existing attachments keep working.",
	"Ready":        "Ready runs once after every instance in the scene exists and references are resolved, in scene pre-order.\n\nfunc Ready() {\n}",
	"Update":       "Update runs every frame. dt is the frame time in seconds.\n\nfunc Update(dt float) {\n}",
	"FixedUpdate":  "FixedUpdate runs every fixed step, before physics. dt is the fixed step, 1/60 by default.\n\nfunc FixedUpdate(dt float) {\n}",
	"Reloaded":     "Reloaded runs after a hot reload for every instance whose script or a dependency changed. Ready does not run again.\n\nfunc Reloaded() {\n}",
	"node":         "node is the NodeRef of the node this instance is attached to. It exists in every script method.\n\nt := node.Transform2D()",
	"bool":         "bool is true or false. Conditions of if and for must be bool.",
	"int":          "int is a signed 64-bit integer that wraps on overflow. Convert with float(i) and int(f); there is no implicit conversion.",
	"float":        "float is a 32-bit IEEE number. float → int conversion saturates and maps NaN to 0.",
	"string":       "string is immutable UTF-8 text. + concatenates, len counts bytes, string(x) converts bool, int and float.",
	"Vec2":         "Vec2 is a 2D vector with lanes X and Y. Build it with a composite literal; it supports +, -, and * or / by a float.\n\nv := Vec2{1, 0} * Speed",
	"Vec3":         "Vec3 is a 3D vector with lanes X, Y and Z.\n\nup := Vec3{0, 1, 0}",
	"Color":        "Color has lanes R, G, B and A in linear space, from 0 to 1.\n\nred := Color{1, 0, 0, 1}",
	"Quat":         "Quat is a 3D rotation with lanes X, Y, Z and W. Build it with math.QuatEuler or math.QuatAxisAngle. Quats cannot be compared with ==.",
	"Rect":         "Rect has lanes X, Y, W and H.",
	"Transform2D":  "Transform2D holds Position Vec2, Rotation float in radians (clockwise) and Scale Vec2. It is a value: read it, change it, write it back.\n\nt := node.Transform2D()\nt.Position.X += 1\nnode.SetTransform2D(t)",
	"Transform3D":  "Transform3D holds Position Vec3, Rotation Quat and Scale Vec3.\n\nt := node.Transform3D()\nt.Position.Y += 1\nnode.SetTransform3D(t)",
	"NodeRef":      "NodeRef references a scene node and may be nil. Exported NodeRef fields are set by dragging a node onto the field in the Inspector.\n\n@export var Target NodeRef",
	"AssetRef":     "AssetRef<Texture | Scene | Prefab | Mesh> references an asset and may be nil. Assign it in the Inspector.\n\n@export var Bullet AssetRef<Prefab>",
	"ComponentRef": "ComponentRef<T> references a component on a node and is re-validated on every access.\n\nsr := node.Get<SpriteRenderer>()\nif sr != nil {\n\tsr.FlipX = true\n}",
	"len":          "len returns the number of elements of a slice or the number of bytes of a string.\n\nn := len(items)",
	"append":       "append returns the slice with v added at the end. Assign the result back.\n\nhits = append(hits, 5)",
	"coroutine":    "coroutine after a method signature makes the method a coroutine: it can pause with yield and continue on a later frame. Only script methods without a result can be coroutines, and lifecycle methods cannot. Run one with start Name(args).\n\nfunc Blink() coroutine {\n\tyield wait(0.1)\n}",
	"start":        "start runs a coroutine method of this script up to its first yield and returns its Coroutine handle. The coroutine belongs to the instance and is cancelled when the instance stops.\n\nblink = start Blink()",
	"yield":        "yield pauses the current coroutine:\n  yield            next frame\n  yield wait(s)    after s seconds of game time\n  yield frames(n)  after n frames\n  yield fixed(n)   after n fixed steps\n  yield c          when coroutine c ends\nyield is allowed only inside a coroutine (AES2050).",
	"yield wait":   "yield wait(seconds float) resumes after the given game time has passed, at the earliest on the next frame.\n\nyield wait(0.25)",
	"yield frames": "yield frames(n int) resumes after n frames; frames(1) is the same as a bare yield.\n\nyield frames(3)",
	"yield fixed":  "yield fixed(n int) resumes after n fixed steps, before physics; use it for movement that must stay in step with FixedUpdate.\n\nyield fixed(1)",
	"signal":       "signal declares an event of the script. Only the script itself can Emit it; other scripts Connect to it through a ScriptRef.\n\nsignal Died(reason string)",
	"Emit":         "Emit calls every connected handler synchronously, in connection order. Connections added or removed by a handler apply after this Emit returns.\n\nDied.Emit(\"fall\")",
	"Connect":      "Connect registers a method with the same parameters as the signal and returns a Connection. The handler must be a plain method: not a coroutine and not a lifecycle method.\n\nconn = enemy.Died.Connect(OnEnemyDied)",
	"Disconnect":   "Disconnect removes the handler; calling it twice is harmless.\n\nconn.Disconnect()",
	"Connected":    "Connected reports whether the connection is still active. It becomes false after Disconnect or when the source or the handler owner stops.",
	"Stop":         "Stop cancels the coroutine; it does not run again. Stopping a finished coroutine is harmless.\n\nblink.Stop()",
	"Running":      "Running reports whether the coroutine is still alive (not finished, cancelled or faulted).\n\nif !blink.Running() {\n\tblink = start Blink()\n}",
	"Coroutine":    "Coroutine is a handle returned by start. The zero value is nil and is never running.",
	"Connection":   "Connection is a handle returned by Connect. Keep it in a field to Disconnect later.",
	"ScriptRef":    "ScriptRef<T> references a script instance on a node. Through it you read and write fields, call plain methods and connect to signals of T. Lifecycle methods and coroutines are not reachable (AES2059). It becomes invalid when the instance stops; access then faults with AES6004.\n\nvar enemy ScriptRef<EnemyAI>",
	"GetScript":    "GetScript<T>() returns the instance of script T on the node, or nil when the node has none. Use pkg.Name when the short name is ambiguous.\n\ne := Target.GetScript<EnemyAI>()\nif e != nil {\n\te.Died.Connect(OnEnemyDied)\n}",
}

func builtinText(key string) string {
	return builtinDocs[key]
}

func objectDoc(o *resolver.Object) string {
	if d := docOf(o); d != "" {
		return d
	}
	switch o.Kind {
	case resolver.ObjType:
		if o.Ident == nil {
			return builtinText(o.Name)
		}
	case resolver.ObjGeneric, resolver.ObjBuiltin, resolver.ObjNode:
		return builtinText(o.Name)
	case resolver.ObjFunc:
		if f, ok := o.Data.(*checker.Func); ok && f.Script != nil {
			return builtinText(f.Name)
		}
	}
	return ""
}

func keywordHover(snap *workspace.Snapshot, file source.FileID, off uint32) (Hover, bool) {
	toks, _, _ := lexer.TokenizeAll(file, snap.Sources.Source(file), source.NewInterner())
	for i, t := range toks {
		if t.Start > off {
			break
		}
		if off > t.End {
			continue
		}
		key := ""
		switch {
		case t.Kind.IsKeyword():
			key = t.Text
		case t.Kind == token.Ident && t.Text == "start" && i+1 < len(toks) && toks[i+1].Kind == token.Ident:
			key = "start"
		case t.Kind == token.Ident && i > 0 && toks[i-1].Kind == token.Yield:
			key = "yield " + t.Text
		}
		if key == "" {
			continue
		}
		if d := builtinText(key); d != "" {
			return Hover{Signature: key, Doc: d, Span: source.Span{File: file, Start: t.Start, End: t.End}}, true
		}
	}
	return Hover{}, false
}
