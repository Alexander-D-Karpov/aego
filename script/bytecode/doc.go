// Package bytecode defines the instruction and module formats shared by the
// compiler, verifier and VM.
//
// Instructions are 32-bit words: ABC is op:8 a:8 b:8 c:8; ABx is op:8 a:8
// bx:16; AsBx uses a signed 16-bit displacement. opInfo defines the format and
// operand roles for both the verifier and disassembler.
//
// Functions use R[0..Registers); parameters begin at R[0], and calls place
// arguments and results in contiguous windows at R[a]. CALL names a local
// function, CALLX an extern resolved by name, CALLH a host import, and RET
// returns b values from R[a]. GETL/SETL access vector lanes; GETCF/SETCF access
// component fields; INDEX/SETINDEX access slices; MOVEN copies consecutive
// slots; VEC2/3/4 pack scalars.
//
// The VM requires unused Value bits to be zero: scalars zero B, floats also
// zero A's upper 32 bits, Vec2 zeros z/w, and Vec3 zeros w. Constructors enforce
// this invariant; GETCF adapters must use them too, since equality compares both
// words or all vector lanes and the verifier cannot check it. EQ compares
// bitwise scalar/reference values; EQF, EQS and EQV compare floats, strings and
// vectors respectively; NE is equality followed by NOT.
//
// ENSURE gives a nil slice its element type before APPEND, whose encoding has no
// type operand. Integer arithmetic wraps modulo 2^64; divide/modulo by zero
// fault with AES6006; MinInt64 / -1 returns MinInt64 and its remainder is zero.
// Floating arithmetic uses IEEE-754 float32 without faults.
//
// Operand widths limit registers and component fields to 255, slice element
// slots to 32, and jump offsets and function size to 32767 instructions. The
// compiler rejects oversized functions rather than emitting an unrepresentable
// jump. DisassembleSource uses the line table and source callback to relate
// bytecode and runtime faults to source.
package bytecode
