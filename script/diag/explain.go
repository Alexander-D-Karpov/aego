package diag

import "sort"

type Help struct {
	What string
	Fix  string
}

var help = map[string]Help{
	UnexpectedChar:      {"This character cannot appear here.", "Use ASCII letters for names, put other text inside a string, and use && or || instead of & or |."},
	UnterminatedString:  {"The string literal has no closing quote.", "Add \" at the end of the string."},
	InvalidEscape:       {"The escape sequence after \\ is not supported.", "Use \\n, \\t, \\\", \\\\ or \\uXXXX."},
	ReservedWord:        {"The word is reserved for a future version of the language.", "Rename the identifier."},
	MalformedNumber:     {"The number literal is malformed.", "Write digits after the point (1.0, not 1.) and separate digits with _ only between digits."},
	UnterminatedComment: {"The block comment has no closing */.", "Add */ where the comment should end."},
	InvalidUTF8:         {"The file contains bytes that are not valid UTF-8.", "Save the file as UTF-8."},
	NewlineInString:     {"A string literal cannot span lines.", "Close the string on the same line or use \\n."},
	UnexpectedToken:     {"The parser expected something else at this point.", "Check for a missing bracket, comma or operator just before this position."},
	MissingPackage:      {"Every script file starts with a package clause.", "Add `package <directory name>` as the first line."},
	MisplacedAttr:       {"Attributes are allowed only on scripts and script fields.", "Move the attribute above a script or a `var` inside a script, or remove it."},
	LateImport:          {"Imports must come before any declaration.", "Move the import to the top of the file, after the package clause."},
	PackageVar:          {"Packages cannot hold mutable state.", "Use const for fixed values, or move the variable into a script as a field."},
	DeclInFunction:      {"Types, functions and constants are declared at package level.", "Move the declaration out of the function body."},
	BadShortVar:         {"The left side of := must be one new name.", "Write `name := value`, or use = to assign to an existing field or element."},
	BadCompositeKey:     {"A composite literal key must be a field name.", "Write `Type{Field: value}`."},
	TooDeep:             {"The code is nested too deeply.", "Split the expression or block into helper functions."},
	BadForCondition:     {"The single part of a for loop must be a condition.", "Write `for cond {}` or `for init; cond; post {}`."},
	ExpectedComposit:    {"A slice type here must be followed by a literal.", "Write `[]T{...}`."},
	MultiAssign:         {"Only one value can be assigned per statement.", "Split the assignment into several statements."},
	ElseOnNewLine:       {"else must follow the closing brace on the same line.", "Write `} else {`."},
	UndefinedName:       {"The name is not declared in this scope.", "Check the spelling, declare it, or add the missing import."},
	NotAType:            {"The name refers to something that is not a type.", "Use a type name such as int, Vec2 or a declared struct."},
	TypeMismatch:        {"The value has a different type than the place it is used.", "Convert explicitly, for example float(x) or int(f)."},
	InvalidOperation:    {"The operator is not defined for these operand types.", "Convert the operands to a type the operator supports."},
	ArgCount:            {"The call passes a different number of arguments than the function expects.", "Match the function signature; hover the function to see it."},
	NotCallable:         {"The expression is not a function.", "Remove the parentheses or call a function instead."},
	NotAssignable:       {"The left side cannot be assigned to.", "Assign to a variable, field, slice element or writable component field."},
	UnknownField:        {"The type has no field or member with this name.", "Check the spelling; press Ctrl+Space after . to see the members."},
	NotExported:         {"The name starts with a lowercase letter, so it is private to its package.", "Rename it to start with an uppercase letter if other packages need it."},
	NotConstant:         {"A constant needs a value known at compile time.", "Use literals and other constants only."},
	DivisionByZero:      {"Integer division by a constant zero.", "Check the divisor."},
	ConstOverflow:       {"The constant does not fit its type.", "Use a smaller value or a wider type."},
	MissingReturn:       {"The function can reach its end without returning a value.", "Add a return statement on every path."},
	MisplacedBranch:     {"break or continue is outside a loop or switch.", "Move it inside a for loop, or use return."},
	NotExhaustive:       {"A switch on an enum must handle every member.", "Add the missing cases or a default."},
	DuplicateCase:       {"The same case value appears twice.", "Remove the duplicate case."},
	RecursiveStruct:     {"A struct cannot contain itself by value.", "Store the relation in a slice or remove the field."},
	Redeclared:          {"The name is already declared in this scope.", "Rename one of the declarations or remove one of them."},
	ReturnMismatch:      {"The returned value does not match the function result.", "Return a value of the declared type, or none in a function without a result."},
	NilMisuse:           {"nil has no type of its own here.", "Give the variable an explicit reference type: `var r NodeRef = nil`."},
	BadTypeArg:          {"The type argument is not valid here.", "Use Get<T>/Has<T> with a component name, AssetRef<Texture|Scene|Prefab|Mesh>."},
	Lifecycle:           {"A lifecycle method has the wrong signature.", "Use func Ready(), func Update(dt float), func FixedUpdate(dt float), func Reloaded()."},
	BadConversion:       {"These types cannot be converted into each other.", "Convert through int, float or string as documented."},
	NotValue:            {"The expression does not produce a value.", "Call the function, or use a value instead of a type or package."},
	Unused:              {"The value is computed and then thrown away.", "Assign the result or remove the expression."},
	ConstCycle:          {"Constants refer to each other in a cycle.", "Break the cycle with a literal value."},
	BadConstType:        {"Constants can only be bool, int, float, string or enum.", "Use a script field for other types."},
	InvalidComposite:    {"The composite literal does not fit its type.", "Use all keyed or all positional values, with the right count."},
	RegisterLimit:       {"The function is too large to compile.", "Split it into smaller functions."},
	CompileBlocked:      {"The package was not built because a dependency has errors.", "Fix the errors in the named package."},
	UnusedVar:           {"The variable is declared but never read.", "Use it or remove it."},
	SliceTooWide:        {"Slice elements may occupy at most 32 value slots.", "Make the element struct smaller or store several slices."},
	NotExportable:       {"Only scalar, vector, enum and reference fields can be exported.", "Remove @export or change the field type."},
	UnknownAttr:         {"The attribute is not known.", "Use @export, @former, @hide or @pause."},
	BadAttrArg:          {"The attribute argument is not valid.", "See the attribute in the Script Reference; hover it for the accepted arguments."},
	DuplicateAttr:       {"The attribute or argument is given twice.", "Keep one."},
	AttrTarget:          {"The attribute does not apply to this declaration.", "Move it to a script or field, or remove it."},
	FieldInit:           {"A field default must be a constant.", "Use a literal or constant, or set the value in Ready."},
	PackageName:         {"The package clause must match the directory name.", "Rename the package clause or move the file."},
	UnknownImport:       {"No script package or engine module has this path.", "Check the path; script packages are directories under scripts/."},
	DuplicateImport:     {"The package is imported twice.", "Remove one import."},
	SelfImport:          {"A package cannot import itself.", "Remove the import."},
	ImportCycle:         {"Packages import each other in a cycle.", "Move the shared code into a third package."},
	DuplicateDecl:       {"The name is declared twice.", "Rename or remove one declaration."},
	ImportConflict:      {"An import uses the same name as a declaration.", "Rename the declaration."},
	UnusedImport:        {"The import is not used.", "Remove it."},
	ReservedPackage:     {"Package paths under aego/ belong to the engine.", "Change the project module."},
	ModuleConflict:      {"A script package and an engine module share the same path.", "Rename the script directory."},
	UnknownHost:         {"The engine or native module has no such member.", "Check the name; press Ctrl+Space after the module name to list members."},
	BadBytecode:         {"A compiled artifact is corrupt.", "Reimport the scripts (Assets → Reimport)."},
	CodeBudget:          {"The callback ran more instructions than its budget, usually an endless loop.", "Make sure every loop ends within one frame."},
	CodeNativePanic:     {"A native Go binding panicked.", "Fix the Go function; the Console shows its stack."},
	CodeCallDepth:       {"Calls nested more than 256 levels, usually endless recursion or re-emitting a signal from its handler.", "Add a stopping condition."},
	CodeNil:             {"A reference is nil or its node, component or script instance no longer exists.", "Check `if ref != nil` and IsAlive before use."},
	CodeIndex:           {"The slice index is outside the slice.", "Check the index against len(slice)."},
	CodeDivZero:         {"Integer division by zero at runtime.", "Check the divisor before dividing."},
	CodeBadHandle:       {"A string or slice handle is invalid.", "Report this as an engine bug."},
	CodeExplicit:        {"A function reached its end without returning.", "Add the missing return."},
	CodeLink:            {"A call or binding could not be linked.", "Rebuild the scripts and native code."},
	CodeHost:            {"An engine or native function reported an error.", "Read the message; it names the cause."},
	YieldOutside:        {"yield suspends a coroutine, so it is allowed only inside a function declared as coroutine.", "Declare the method as func Name() coroutine and start it with start Name()."},
	CoroutineCall:       {"A coroutine cannot be called like a plain function.", "Write start Name(args); keep the result in a Coroutine field to stop it later."},
	StartTarget:         {"start runs a coroutine method of the current script.", "Mark the method with coroutine after its signature, or call a plain method directly."},
	CoroutineDecl:       {"A coroutine is a script method and returns no value.", "Move it into a script and remove the result type; pass results through fields or signals."},
	HandlerMismatch:     {"The handler parameters must match the signal parameters exactly.", "Change the handler to take the same types in the same order as the signal."},
	BadConnect:          {"Connect takes a plain method of this script or ref.Method; signals are emitted only by their own script.", "Pass a method name like OnDied or other.OnDied; call Emit only on a signal of this script."},
	CoroutineLifecycle:  {"Ready, Update, FixedUpdate and Reloaded are called by the engine and cannot be coroutines.", "Keep the lifecycle method plain and write start Name() inside it."},
	YieldArg:            {"wait and waitUnscaled take seconds as float, frames and fixed take a count as int, a bare yield value must be a Coroutine.", "Convert the value, for example yield wait(float(n)) or yield frames(int(t))."},
	ScriptRefTarget:     {"ScriptRef and GetScript need the name of a script type.", "Use the script name, or pkg.Name when the short name is ambiguous."},
	ScriptRefCall:       {"Lifecycle methods and coroutines run only on their own instance.", "Call a plain method of the target that does the work, or connect to a signal."},
	EventSignature:      {"A script method named after an engine event receives that event and must match its signature.", "Use the parameter types shown in the hover for the event, or rename the method if it is not meant to be a handler."},
	EventCall:           {"Engine event handlers are called only by the engine.", "Move the shared code into a plain method and call that from the handler and from your code."},
}

func Explain(code string) (Help, bool) {
	h, ok := help[code]
	return h, ok
}

func HelpText(code string) string {
	if h, ok := help[code]; ok {
		return h.What + " Fix: " + h.Fix
	}
	return ""
}

func Codes() []string {
	out := make([]string, 0, len(help))
	for c := range help {
		out = append(out, c)
	}
	sort.Strings(out)
	return out
}
