package diag

import "strings"

// File is an extra source file of an example, Path is relative to scripts/.
type File struct {
	Path string
	Bad  string
	Good string
}

type Ex struct {
	Name  string
	Bad   string
	Good  string
	Files []File
	Why   string
}

// MainFile is the path of an example's Bad and Good under scripts/.
func (e Ex) MainFile() string {
	if e.Name != "" {
		return e.Name
	}
	return "demo/main.aes"
}

func (e Ex) Sources(good bool) map[string]string {
	out := map[string]string{e.MainFile(): e.Bad}
	if good {
		out[e.MainFile()] = e.Good
	}
	for _, f := range e.Files {
		out[f.Path] = f.Bad
		if good {
			out[f.Path] = f.Good
		}
	}
	return out
}

func Example(code string) (Ex, bool) {
	e, ok := examples[code]
	return e, ok
}

var phases = []struct{ from, to, name string }{
	{"AES1001", "AES1099", "Lexer"},
	{"AES1100", "AES1199", "Parser"},
	{"AES2001", "AES2049", "Types"},
	{"AES2050", "AES2099", "Coroutines, signals and events"},
	{"AES3001", "AES3999", "Attributes"},
	{"AES4001", "AES4999", "Packages and imports"},
	{"AES5001", "AES5999", "Engine bindings"},
	{"AES6001", "AES6099", "Runtime faults"},
	{"AES6100", "AES6999", "Compiled code"},
}

func Phase(code string) string {
	for _, p := range phases {
		if code >= p.from && code <= p.to {
			return p.name
		}
	}
	return "Other"
}

func deepConst() string {
	return "package demo\n\nconst Answer = " + strings.Repeat("(", 520) + "42" + strings.Repeat(")", 520) + "\n"
}

var examples = map[string]Ex{
	UnexpectedChar: {
		Bad: `package demo

func Both(a bool, b bool) bool {
	return a & b
}
`,
		Good: `package demo

func Both(a bool, b bool) bool {
	return a && b
}
`,
	},
	UnterminatedString: {
		Bad:  "package demo\n\nconst Greeting = \"hello",
		Good: "package demo\n\nconst Greeting = \"hello\"\n",
	},
	InvalidEscape: {
		Bad: `package demo

const Folder = "saves\data"
`,
		Good: `package demo

const Folder = "saves\\data"
`,
	},
	ReservedWord: {
		Bad: `package demo

func Pick(n int) int {
	select := n * 2
	return select
}
`,
		Good: `package demo

func Pick(n int) int {
	choice := n * 2
	return choice
}
`,
	},
	MalformedNumber: {
		Bad: `package demo

const Speed = 2.
`,
		Good: `package demo

const Speed = 2.0
`,
	},
	UnterminatedComment: {
		Bad: `package demo

/* TODO: tune the speed
const Speed = 2.0
`,
		Good: `package demo

/* TODO: tune the speed */
const Speed = 2.0
`,
	},
	InvalidUTF8: {
		Bad:  "package demo\n\nconst Name = \"Caf\xe9\"\n",
		Good: "package demo\n\nconst Name = \"Café\"\n",
	},
	NewlineInString: {
		Bad:  "package demo\n\nconst Hint = \"Press Space\nto jump\"\n",
		Good: "package demo\n\nconst Hint = \"Press Space\\nto jump\"\n",
	},
	UnexpectedToken: {
		Bad: `package demo

func Area(w float, h float) float {
	return (w * h
}
`,
		Good: `package demo

func Area(w float, h float) float {
	return (w * h)
}
`,
	},
	MissingPackage: {
		Bad: `func Twice(n int) int {
	return n * 2
}
`,
		Good: `package demo

func Twice(n int) int {
	return n * 2
}
`,
	},
	MisplacedAttr: {
		Bad: `package demo

@export
const Speed = 4.0
`,
		Good: `package demo

script Mover {
	@export var Speed float = 4.0
}
`,
	},
	LateImport: {
		Bad: `package demo

const Name = "demo"

import "aego/log"

func Hello() {
	log.Info(Name)
}
`,
		Good: `package demo

import "aego/log"

const Name = "demo"

func Hello() {
	log.Info(Name)
}
`,
	},
	PackageVar: {
		Bad: `package demo

var score int

func Add(n int) {
	score = score + n
}
`,
		Good: `package demo

script Score {
	var score int

	func Add(n int) {
		score = score + n
	}
}
`,
	},
	DeclInFunction: {
		Bad: `package demo

func Clamp(n int) int {
	const Max = 10
	if n > Max {
		return Max
	}
	return n
}
`,
		Good: `package demo

const Max = 10

func Clamp(n int) int {
	if n > Max {
		return Max
	}
	return n
}
`,
	},
	BadShortVar: {
		Bad: `package demo

type Stats struct {
	Hp int
}

func Heal(s Stats) Stats {
	s.Hp := 10
	return s
}
`,
		Good: `package demo

type Stats struct {
	Hp int
}

func Heal(s Stats) Stats {
	s.Hp = 10
	return s
}
`,
	},
	BadCompositeKey: {
		Bad: `package demo

type Stats struct {
	Hp int
}

func Fresh() Stats {
	return Stats{"Hp": 10}
}
`,
		Good: `package demo

type Stats struct {
	Hp int
}

func Fresh() Stats {
	return Stats{Hp: 10}
}
`,
	},
	TooDeep: {
		Bad:  deepConst(),
		Good: "package demo\n\nconst Answer = 42\n",
	},
	BadForCondition: {
		Bad: `package demo

func Sum() int {
	total := 0
	for i := 0 {
		total = total + i
	}
	return total
}
`,
		Good: `package demo

func Sum() int {
	total := 0
	for i := 0; i < 3; i++ {
		total = total + i
	}
	return total
}
`,
	},
	ExpectedComposit: {
		Bad: `package demo

func Empty() int {
	xs := []int
	return len(xs)
}
`,
		Good: `package demo

func Empty() int {
	xs := []int{}
	return len(xs)
}
`,
	},
	MultiAssign: {
		Bad: `package demo

func Order(a int, b int) int {
	a, b = b, a
	return a - b
}
`,
		Good: `package demo

func Order(a int, b int) int {
	t := a
	a = b
	b = t
	return a - b
}
`,
	},
	ElseOnNewLine: {
		Bad: `package demo

func Abs(n int) int {
	if n < 0 {
		return -n
	}
	else {
		return n
	}
}
`,
		Good: `package demo

func Abs(n int) int {
	if n < 0 {
		return -n
	} else {
		return n
	}
}
`,
	},
	UndefinedName: {
		Bad: `package demo

func Area(width int, height int) int {
	return width * heigth
}
`,
		Good: `package demo

func Area(width int, height int) int {
	return width * height
}
`,
	},
	NotAType: {
		Bad: `package demo

const Lives = 3

func Start() int {
	var n Lives
	return n
}
`,
		Good: `package demo

const Lives = 3

func Start() int {
	var n int = Lives
	return n
}
`,
	},
	TypeMismatch: {
		Bad: `package demo

func Half(n int) float {
	return n / 2
}
`,
		Good: `package demo

func Half(n int) float {
	return float(n) / 2
}
`,
	},
	InvalidOperation: {
		Bad: `package demo

func Alive(hp int) bool {
	return !hp
}
`,
		Good: `package demo

func Alive(hp int) bool {
	return hp > 0
}
`,
	},
	ArgCount: {
		Bad: `package demo

func Damage(hp int, amount int) int {
	return hp - amount
}

func Hit() int {
	return Damage(10)
}
`,
		Good: `package demo

func Damage(hp int, amount int) int {
	return hp - amount
}

func Hit() int {
	return Damage(10, 3)
}
`,
	},
	NotCallable: {
		Bad: `package demo

func Fast() float {
	speed := 2.0
	return speed() * 2
}
`,
		Good: `package demo

func Fast() float {
	speed := 2.0
	return speed * 2
}
`,
	},
	NotAssignable: {
		Bad: `package demo

const Lives = 3

func Reset() {
	Lives = 5
}
`,
		Good: `package demo

script Game {
	var lives int = 3

	func Reset() {
		lives = 5
	}
}
`,
	},
	UnknownField: {
		Bad: `package demo

func Height(v Vec2) float {
	return v.Z
}
`,
		Good: `package demo

func Height(v Vec2) float {
	return v.Y
}
`,
	},
	NotExported: {
		Bad: `package demo

import "game/rules"

func Lives() int {
	return rules.startLives
}
`,
		Good: `package demo

import "game/rules"

func Lives() int {
	return rules.StartLives
}
`,
		Files: []File{{
			Path: "rules/rules.aes",
			Bad:  "package rules\n\nconst startLives = 3\n",
			Good: "package rules\n\nconst StartLives = 3\n",
		}},
	},
	NotConstant: {
		Bad: `package demo

func Base() int {
	return 10
}

const Max = Base() * 2
`,
		Good: `package demo

const Base = 10

const Max = Base * 2
`,
	},
	DivisionByZero: {
		Bad: `package demo

const Steps = 0

const Size = 100 / Steps
`,
		Good: `package demo

const Steps = 4

const Size = 100 / Steps
`,
	},
	ConstOverflow: {
		Bad: `package demo

const Huge = 9223372036854775808
`,
		Good: `package demo

const Huge = 9223372036854775807
`,
	},
	MissingReturn: {
		Bad: `package demo

func Sign(n int) int {
	if n < 0 {
		return -1
	}
}
`,
		Good: `package demo

func Sign(n int) int {
	if n < 0 {
		return -1
	}
	return 1
}
`,
	},
	MisplacedBranch: {
		Bad: `package demo

func Check(n int) int {
	if n > 10 {
		break
	}
	return n
}
`,
		Good: `package demo

func Check(n int) int {
	if n > 10 {
		return 10
	}
	return n
}
`,
	},
	NotExhaustive: {
		Bad: `package demo

enum Dir {
	Left
	Right
}

func Sign(d Dir) int {
	switch d {
	case Dir.Left:
		return -1
	}
	return 0
}
`,
		Good: `package demo

enum Dir {
	Left
	Right
}

func Sign(d Dir) int {
	switch d {
	case Dir.Left:
		return -1
	case Dir.Right:
		return 1
	}
	return 0
}
`,
	},
	DuplicateCase: {
		Bad: `package demo

func Name(n int) string {
	switch n {
	case 1:
		return "one"
	case 1:
		return "two"
	}
	return "many"
}
`,
		Good: `package demo

func Name(n int) string {
	switch n {
	case 1:
		return "one"
	case 2:
		return "two"
	}
	return "many"
}
`,
	},
	RecursiveStruct: {
		Bad: `package demo

type Room struct {
	Name string
	Next Room
}
`,
		Good: `package demo

type Room struct {
	Name string
	Next []Room
}
`,
	},
	Redeclared: {
		Bad: `package demo

func Total() int {
	sum := 1
	sum := 2
	return sum
}
`,
		Good: `package demo

func Total() int {
	sum := 1
	sum = 2
	return sum
}
`,
	},
	ReturnMismatch: {
		Bad: `package demo

func Reset() {
	return 0
}
`,
		Good: `package demo

func Reset() {
	return
}
`,
	},
	NilMisuse: {
		Bad: `package demo

script Follow {
	func Ready() {
		target := nil
		if target == nil {
			node.Destroy()
		}
	}
}
`,
		Good: `package demo

script Follow {
	func Ready() {
		var target NodeRef = nil
		if target == nil {
			node.Destroy()
		}
	}
}
`,
	},
	BadTypeArg: {
		Bad: `package demo

script Probe {
	func Ready() {
		if node.Has<Health>() {
			node.Destroy()
		}
	}
}
`,
		Good: `package demo

script Probe {
	func Ready() {
		if node.Has<SpriteRenderer>() {
			node.Destroy()
		}
	}
}
`,
	},
	Lifecycle: {
		Bad: `package demo

script Clock {
	var elapsed float

	func Update() {
		elapsed = elapsed + 0.016
	}
}
`,
		Good: `package demo

script Clock {
	var elapsed float

	func Update(dt float) {
		elapsed = elapsed + dt
	}
}
`,
	},
	BadConversion: {
		Bad: `package demo

func Flag(n int) bool {
	return bool(n)
}
`,
		Good: `package demo

func Flag(n int) bool {
	return n != 0
}
`,
	},
	NotValue: {
		Bad: `package demo

func Zero() int {
	x := int
	return x
}
`,
		Good: `package demo

func Zero() int {
	x := 0
	return x
}
`,
	},
	Unused: {
		Bad: `package demo

func Next(n int) int {
	n + 1
	return n
}
`,
		Good: `package demo

func Next(n int) int {
	n = n + 1
	return n
}
`,
	},
	ConstCycle: {
		Bad: `package demo

const Width = Height * 2
const Height = Width / 2
`,
		Good: `package demo

const Width = 64
const Height = Width / 2
`,
	},
	BadConstType: {Why: "Every other constant type is rejected earlier as AES2010 or AES2003."},
	InvalidComposite: {
		Bad: `package demo

type Point struct {
	X int
	Y int
}

func Origin() Point {
	return Point{0, 0, 0}
}
`,
		Good: `package demo

type Point struct {
	X int
	Y int
}

func Origin() Point {
	return Point{0, 0}
}
`,
	},
	RegisterLimit: {
		Bad: `package demo

type Pose struct {
	A Transform2D
	B Transform2D
	C Transform2D
	D Transform2D
}

type Rig struct {
	Head Pose
	Body Pose
	Arms Pose
	Legs Pose
	Tail Pose
	Wing Pose
}

func Blend(a Rig, b Rig, c Rig, d Rig) Rig {
	return a
}
`,
		Good: `package demo

type Pose struct {
	A Transform2D
	B Transform2D
	C Transform2D
	D Transform2D
}

type Rig struct {
	Head Pose
	Body Pose
	Arms Pose
	Legs Pose
	Tail Pose
	Wing Pose
}

func Blend(a Rig, b Rig) Rig {
	return a
}
`,
	},
	CompileBlocked: {
		Bad: `package demo

import "game/rules"

func Lives() int {
	return rules.Lives
}
`,
		Good: `package demo

import "game/rules"

func Lives() int {
	return rules.Lives
}
`,
		Files: []File{{
			Path: "rules/rules.aes",
			Bad:  "package rules\n\nconst Lives = \"three\" * 1\n",
			Good: "package rules\n\nconst Lives = 3\n",
		}},
	},
	UnusedVar: {
		Bad: `package demo

func Score(hits int) int {
	bonus := hits * 10
	return hits
}
`,
		Good: `package demo

func Score(hits int) int {
	bonus := hits * 10
	return hits + bonus
}
`,
	},
	SliceTooWide: {
		Bad: `package demo

type Pose struct {
	A Transform2D
	B Transform2D
	C Transform2D
	D Transform2D
}

type Frame struct {
	Hips Pose
	Arms Pose
	Legs Pose
}

func Count() int {
	frames := []Frame{}
	return len(frames)
}
`,
		Good: `package demo

type Pose struct {
	A Transform2D
	B Transform2D
	C Transform2D
	D Transform2D
}

func Count() int {
	hips := []Pose{}
	arms := []Pose{}
	return len(hips) + len(arms)
}
`,
	},
	YieldOutside: {
		Bad: `package demo

script Blink {
	func Ready() {
		yield wait(0.5)
		node.Destroy()
	}
}
`,
		Good: `package demo

script Blink {
	func Ready() {
		start Later()
	}

	func Later() coroutine {
		yield wait(0.5)
		node.Destroy()
	}
}
`,
	},
	CoroutineCall: {
		Bad: `package demo

script Blink {
	func Ready() {
		Later()
	}

	func Later() coroutine {
		yield wait(0.5)
		node.Destroy()
	}
}
`,
		Good: `package demo

script Blink {
	func Ready() {
		start Later()
	}

	func Later() coroutine {
		yield wait(0.5)
		node.Destroy()
	}
}
`,
	},
	StartTarget: {
		Bad: `package demo

script Blink {
	func Ready() {
		start Vanish()
	}

	func Vanish() {
		node.Destroy()
	}
}
`,
		Good: `package demo

script Blink {
	func Ready() {
		start Vanish()
	}

	func Vanish() coroutine {
		yield wait(0.5)
		node.Destroy()
	}
}
`,
	},
	CoroutineDecl: {
		Bad: `package demo

func Pause() coroutine {
	yield wait(1.0)
}
`,
		Good: `package demo

script Pauser {
	func Pause() coroutine {
		yield wait(1.0)
	}
}
`,
	},
	HandlerMismatch: {
		Bad: `package demo

import "aego/log"

script Health {
	signal Hit(amount int)

	func Ready() {
		Hit.Connect(OnHit)
	}

	func OnHit(amount float) {
		log.Info("hit " + string(amount))
	}
}
`,
		Good: `package demo

import "aego/log"

script Health {
	signal Hit(amount int)

	func Ready() {
		Hit.Connect(OnHit)
	}

	func OnHit(amount int) {
		log.Info("hit " + string(amount))
	}
}
`,
	},
	BadConnect: {
		Bad: `package demo

script Health {
	signal Died()

	func Ready() {
		Died.Connect(Fade)
	}

	func Fade() coroutine {
		yield wait(0.5)
		node.Destroy()
	}
}
`,
		Good: `package demo

script Health {
	signal Died()

	func Ready() {
		Died.Connect(OnDied)
	}

	func OnDied() {
		start Fade()
	}

	func Fade() coroutine {
		yield wait(0.5)
		node.Destroy()
	}
}
`,
	},
	CoroutineLifecycle: {
		Bad: `package demo

script Blink {
	func Ready() coroutine {
		yield wait(0.5)
		node.Destroy()
	}
}
`,
		Good: `package demo

script Blink {
	func Ready() {
		start Vanish()
	}

	func Vanish() coroutine {
		yield wait(0.5)
		node.Destroy()
	}
}
`,
	},
	YieldArg: {
		Bad: `package demo

script Blink {
	func Ready() {
		start Vanish(2)
	}

	func Vanish(seconds int) coroutine {
		yield wait(seconds)
		node.Destroy()
	}
}
`,
		Good: `package demo

script Blink {
	func Ready() {
		start Vanish(2)
	}

	func Vanish(seconds int) coroutine {
		yield wait(float(seconds))
		node.Destroy()
	}
}
`,
	},
	ScriptRefTarget: {
		Bad: `package demo

script Player {
	var enemy ScriptRef<Enemy>
}
`,
		Good: `package demo

script Enemy {
}

script Player {
	var enemy ScriptRef<Enemy>
}
`,
	},
	ScriptRefCall: {
		Bad: `package demo

script Enemy {
	func Ready() {
	}
}

script Player {
	var enemy ScriptRef<Enemy>

	func Respawn() {
		enemy.Ready()
	}
}
`,
		Good: `package demo

script Enemy {
	func Ready() {
		Reset()
	}

	func Reset() {
	}
}

script Player {
	var enemy ScriptRef<Enemy>

	func Respawn() {
		enemy.Reset()
	}
}
`,
	},
	EventSignature: {
		Bad: `package demo

script Pickup {
	func OnTriggerEnter(other int) {
		node.Destroy()
	}
}
`,
		Good: `package demo

script Pickup {
	func OnTriggerEnter(other NodeRef) {
		node.Destroy()
	}
}
`,
	},
	EventCall: {
		Bad: `package demo

script Pickup {
	func OnTriggerEnter(other NodeRef) {
		node.Destroy()
	}

	func Collect() {
		OnTriggerEnter(nil)
	}
}
`,
		Good: `package demo

script Pickup {
	func OnTriggerEnter(other NodeRef) {
		Collect()
	}

	func Collect() {
		node.Destroy()
	}
}
`,
	},
	NotExportable: {
		Bad: `package demo

script Path {
	@export var Points []Vec2
}
`,
		Good: `package demo

script Path {
	@export var Start Vec2
	var points []Vec2
}
`,
	},
	UnknownAttr: {
		Bad: `package demo

@tool
script Grid {
}
`,
		Good: `package demo

script Grid {
}
`,
	},
	BadAttrArg: {
		Bad: `package demo

script Mover {
	@export(min=5, max=1) var Speed int
}
`,
		Good: `package demo

script Mover {
	@export(min=1, max=5) var Speed int
}
`,
	},
	DuplicateAttr: {
		Bad: `package demo

script Mover {
	@export
	@export
	var Speed float
}
`,
		Good: `package demo

script Mover {
	@export
	var Speed float
}
`,
	},
	AttrTarget: {
		Bad: `package demo

@export
script Mover {
	var Speed float
}
`,
		Good: `package demo

script Mover {
	@export var Speed float
}
`,
	},
	FieldInit: {
		Bad: `package demo

func Start() int {
	return 3
}

script Player {
	var lives int = Start()
}
`,
		Good: `package demo

func Start() int {
	return 3
}

script Player {
	var lives int

	func Ready() {
		lives = Start()
	}
}
`,
	},
	NotPersistable: {
		Bad: `package demo

script Inventory {
	@persist var items []string
}
`,
		Good: `package demo

script Inventory {
	@persist var gold int
	var items []string
}
`,
	},
	PackageName: {
		Name: "level/main.aes",
		Bad: `package demo

const Width = 64
`,
		Good: `package level

const Width = 64
`,
	},
	UnknownImport: {
		Bad: `package demo

import "aego/sound"

func Beep() {
	sound.Play()
}
`,
		Good: `package demo

import "aego/log"

func Beep() {
	log.Info("beep")
}
`,
	},
	DuplicateImport: {
		Bad: `package demo

import (
	"aego/log"
	"aego/log"
)

func Hello() {
	log.Info("hello")
}
`,
		Good: `package demo

import "aego/log"

func Hello() {
	log.Info("hello")
}
`,
	},
	SelfImport: {
		Bad: `package demo

import "game/demo"

const Width = 64
`,
		Good: `package demo

const Width = 64
`,
	},
	ImportCycle: {
		Bad: `package demo

import "game/rules"

const Lives = rules.Lives

const Name = "demo"
`,
		Good: `package demo

import "game/rules"

const Lives = rules.Lives

const Name = "demo"
`,
		Files: []File{{
			Path: "rules/rules.aes",
			Bad:  "package rules\n\nimport \"game/demo\"\n\nconst Lives = 3\n\nconst Title = demo.Name\n",
			Good: "package rules\n\nconst Lives = 3\n",
		}},
	},
	DuplicateDecl: {
		Bad: `package demo

const Speed = 1.0

const Speed = 2.0
`,
		Good: `package demo

const Speed = 1.0

const RunSpeed = 2.0
`,
	},
	ImportConflict: {
		Bad: `package demo

import "aego/log"

const log = 1

func Hello() {
	log.Info("hello")
}
`,
		Good: `package demo

import "aego/log"

const logLevel = 1

func Hello() {
	log.Info("hello")
}
`,
	},
	UnusedImport: {
		Bad: `package demo

import "aego/log"

const Width = 64
`,
		Good: `package demo

const Width = 64
`,
	},
	ReservedPackage: {Why: "Needs a project whose module path starts with aego/; projects use their own module."},
	ModuleConflict:  {Why: "Needs a native module in the project with the same path as a script directory."},
	UnknownHost: {
		Bad: `package demo

import "aego/log"

func Hello() {
	log.Shout("hello")
}
`,
		Good: `package demo

import "aego/log"

func Hello() {
	log.Info("hello")
}
`,
	},
	BadBytecode: {Why: "Reported when a compiled artifact on disk is damaged, not by source code."},
	CodeBudget: {
		Bad: `package demo

script Spinner {
	var turns int

	func Update(dt float) {
		for turns >= 0 {
			turns++
		}
	}
}
`,
		Good: `package demo

script Spinner {
	var turns int

	func Update(dt float) {
		for turns < 10 {
			turns++
		}
	}
}
`,
	},
	CodeNativePanic: {Why: "Raised when the Go code behind a native binding panics."},
	CodeCallDepth: {
		Bad: `package demo

func Count(n int) int {
	return Count(n + 1)
}

script Counter {
	var total int

	func Ready() {
		total = Count(0)
	}
}
`,
		Good: `package demo

func Count(n int) int {
	if n >= 10 {
		return n
	}
	return Count(n + 1)
}

script Counter {
	var total int

	func Ready() {
		total = Count(0)
	}
}
`,
	},
	CodeNil: {
		Bad: `package demo

script Chaser {
	var target ScriptRef<Chaser>

	func Ready() {
		target.Chase()
	}

	func Chase() {
	}
}
`,
		Good: `package demo

script Chaser {
	var target ScriptRef<Chaser>

	func Ready() {
		if target != nil {
			target.Chase()
		}
	}

	func Chase() {
	}
}
`,
	},
	CodeIndex: {
		Bad: `package demo

script Waves {
	var sizes []int
	var last int

	func Ready() {
		sizes = []int{3, 5, 8}
		last = sizes[3]
	}
}
`,
		Good: `package demo

script Waves {
	var sizes []int
	var last int

	func Ready() {
		sizes = []int{3, 5, 8}
		last = sizes[len(sizes)-1]
	}
}
`,
	},
	CodeDivZero: {
		Bad: `package demo

script Stats {
	var kills int
	var deaths int
	var ratio int

	func Ready() {
		ratio = kills / deaths
	}
}
`,
		Good: `package demo

script Stats {
	var kills int
	var deaths int
	var ratio int

	func Ready() {
		if deaths > 0 {
			ratio = kills / deaths
		}
	}
}
`,
	},
	CodeBadHandle: {Why: "Means the engine passed a damaged string or slice handle; report it as a bug."},
	CodeExplicit:  {Why: "The compiler reports a missing return as AES2013 before the code can run."},
	CodeLink:      {Why: "Raised when compiled scripts do not match the engine or native code they were built against."},
	CodeHost: {
		Bad: `package demo

import "aego/scene"

script Gun {
	var Bullet AssetRef<Prefab>

	func Ready() {
		scene.Instantiate(Bullet, nil)
	}
}
`,
		Good: `package demo

import "aego/scene"

script Gun {
	var Bullet AssetRef<Prefab>

	func Ready() {
		if Bullet != nil {
			scene.Instantiate(Bullet, nil)
		}
	}
}
`,
	},
}
