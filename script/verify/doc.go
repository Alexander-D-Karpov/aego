// Package verify statically checks bytecode (D99): OpInfo operand ranges, call windows against register counts, in-function jumps and terminal instructions at function ends.
// It checks monotonic line tables, script field layout/GC-root (HeapSlots) consistency, unique names and encoding limits; all module slice accesses are bounds-checked.
//
// # Checks left to execution or linking
//
//   - INDEX / SETINDEX / APPEND: the VM checks the index against slice length and the value window a..a+width against Registers, using the heap header's element width.
//   - GETCF / SETCF: the VM checks the field index against the component table of the runtime reference in register b.
//   - CALLX: linking matches the extern's Params/Results against the target function in the other module.
//   - CALLR / GETXF / SETXF / CONNECT: vm.Link resolves the other module's method kind (not lifecycle or coroutine), field layout and signal/handler pairing by name, rejecting mismatches.
package verify
