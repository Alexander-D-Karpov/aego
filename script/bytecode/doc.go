// Package bytecode defines the Aego Script instruction set, the compiled module
// format, and the encoder/decoder and disassembler over them. It is the shared
// contract between the compiler (which emits it), the verifier (which proves it
// safe before execution), and the VM (which runs it).
//
// # Instruction encoding
//
// Each instruction is a 32-bit word, Lua-style, in one of three formats:
//
//	op:8  a:8  b:8  c:8      (FormatABC)
//	op:8  a:8  bx:16         (FormatABx)
//	op:8  a:8  sbx:16        (FormatAsBx, sbx signed)
//
// opInfo is the single source of truth for each opcode's format and the kind of
// its operands; the disassembler and verifier both read it, so an opcode is
// described in exactly one place.
//
// # Registers and calling convention
//
// A function runs in a window of R[0..Registers). Parameters arrive in R[0..n).
// A call lays its arguments contiguously above the callee-visible frame:
//
//	CALL  a bx   calls function bx with arguments in R[a..a+params) and leaves
//	             results in R[a..a+results).
//	CALLX a bx   same, but bx indexes the extern table (another package's
//	             function, resolved by name at link time, not by index).
//	CALLH a bx   same, but bx indexes the host-import table.
//	RET   a b    returns b values starting at R[a].
//
// Operand roles that the encoding alone does not make obvious:
//
//	GETL     a b c   a = b's lane c             (c is a lane index 0..3 → x,y,z,w)
//	SETL     a b c   a's lane c = scalar b
//	GETCF    a b c   a = field c of component-ref b
//	SETCF    a b c   field c of component-ref b = a
//	INDEX    a b c   a = b[c]                    (b slice, c index)
//	SETINDEX a b c   a[b] = c                    (a slice, b index, c value base)
//	MOVEN    a b c   copy c consecutive slots R[b..b+c) to R[a..a+c) (flat structs)
//	VEC2/3/4 a b     pack consecutive scalars at R[b..] into the vector in R[a]
//
// # Zero-bit invariant (load-bearing, not checked by the verifier)
//
// vm.Value is two words {A, B}. EQ compares *both* words and EQV compares all
// four lanes, so every instruction and adapter that writes a register must
// leave the unused bits zero — this invariant is what lets one EQ serve every
// one- and two-word scalar/reference type:
//
//   - a scalar (int, bool, enum, NodeRef) writes A and must zero B;
//   - a float writes the low 32 bits of A and must zero the high 32 bits and B;
//   - Vec2 fills lanes x,y and must zero z,w; Vec3 must zero w.
//
// The constructors (LOADI/LOADK, VEC2-4, ITOF/FTOI, LEN, …) satisfy it, and
// GETCF writes whatever Comps.GetField returns — so the bind adapters must build
// their values through those constructors, never by stuffing raw bits. The
// verifier does not police this; a VEC3-then-copy-as-Vec2 that left a stale z
// would make `v == Vec2{1,2}` spuriously false. Keep it in mind for every write.
//
// # Equality
//
// Because the invariant guarantees unused words are zero, one EQ covers every
// value whose identity is its bits:
//
//	EQ   both words A,B  int, bool, enum, NodeRef, AssetRef, ComponentRef
//	EQF  IEEE float32    the low 32 bits of A
//	EQS  string contents
//	EQV  four lanes by IEEE  Vec2/Vec3/Color/Rect (Quat is not comparable; q ≡ −q)
//
// AssetRef (a 128-bit id) and ComponentRef (NodeHandle + index) live in both
// words, which EQ already compares; no separate two-word opcode is needed. NE is
// compiled as EQ-family + NOT.
//
// # Slices and nil
//
// The zero value of a slice is nil, which has no element type. APPEND is an ABC
// instruction with no room for a type operand, so it cannot allocate a typed
// backing object on its own. ENSURE r t therefore runs before the first APPEND
// to a possibly-nil slice: it carries the element type (ABx) and replaces a nil
// slice in r with an empty typed object. The alternative — making every slice
// variable and instance field a pre-allocated empty object — costs an
// allocation per slot, so the extra opcode is cheaper.
//
// # Integer arithmetic and faults
//
// Integer arithmetic (ADDI, SUBI, MULI, NEGI) wraps modulo 2^64, like Go.
// DIVI and MODI by zero raise fault AES6003 rather than crashing the host. To
// stay total, MinInt64 / -1 is defined as MinInt64 and MinInt64 % -1 as 0 (Go
// panics on the former; the VM must not). Floating arithmetic is plain IEEE-754
// float32 with no faults.
//
// # Encoding-derived limits
//
// See the Max* constants. MaxRegisters (255) and MaxComponentFields (255) follow
// from uint8 operands; MaxSliceElemSlots (32) from the uint32 TypeDef.Mask;
// MaxJumpOffset (32767) from the int16 jump displacement, which also caps a
// function at that many instructions — the compiler must reject a larger
// function with a source-level "function too large" error, since it cannot emit
// a spanning jump.
//
// # Disassembler
//
// Disassemble / DisassembleSource render a module as text. This is not cosmetic:
// it is how compiler output is golden-tested (so a regression reads as
// "ADDF r3 became ADDI r3" with a source line, not "byte 1432 differs"), how the
// compiler-vs-VM question is answered during development (and by `aego dump`),
// and — via the same Function line table that produces the "; file:line source"
// comments — how runtime faults and a future step debugger map back to source.
// The source callback on DisassembleSource exists for that last use.
package bytecode
