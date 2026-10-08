// Package verify statically validates a compiled bytecode.Module before it is
// allowed to run (requirement D99): it checks operand ranges against OpInfo,
// call windows against each function's register count, jumps within their
// function, that every function ends on a terminal instruction, that the line
// table is monotonic, script field layout and GC-root (HeapSlots) consistency,
// name uniqueness, and the encoding-derived limits. It never indexes a module
// slice without a bounds check, so it is safe on fuzzed input.
//
// # Checks it deliberately leaves to the VM at run time
//
// Some invariants depend on values that are only known while executing, so the
// verifier cannot prove them from the artifact alone. The VM must enforce each:
//
//   - INDEX / SETINDEX / APPEND: the element width lives in the heap object's
//     header, not the instruction, so the VM checks the index against the
//     slice's length and that the value window a..a+width stays within
//     Registers.
//   - GETCF / SETCF: the field index is validated against the component table
//     of the component reference held in register b, which is a runtime value.
//   - CALLX: the extern's declared Params/Results are matched against the real
//     target function at link time, when the other package's module is present.
//
// Each is a single comparison, listed here so it stays visible that the static
// verifier has not already covered them.
package verify
