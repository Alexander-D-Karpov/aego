// Package runtime drives compiled script instances during Play: it instantiates
// script attachments, runs the Ready/FixedUpdate/Update lifecycle, routes
// component access, and isolates faults per instance.
//
// # Invariant: the self slice handed to m.Call never goes stale
//
// A call runs against self := t.vals[base : base+slots] — a slice into the
// per-type value arena. The arena only ever grows in create, and create is only
// reached from flush (between phases, before any lifecycle call of the frame),
// never from inside a running call. So while a script is executing, t.vals is
// never reallocated out from under the self slice the VM holds. Everything here
// depends on that: if create could run mid-call (e.g. a host binding that spawns
// an instance synchronously), the executing self slice could alias freed memory.
// Script-driven Instantiate goes through the scene's deferred Flush, so new
// nodes are picked up by a later flush round, not inside the current call.
//
// # Invariant: GC roots are the instance arenas, collected only between phases
//
// MarkRoots walks live instances and marks each arena's heap slots; the VM runs
// mark-sweep only at a safe point with no active frames (end of a phase). Dead
// instances (state == dead or base < 0) are skipped, and their arena slots are
// returned to the per-type free list and instance id to instFree in compact,
// which likewise runs only between phases.
package runtime
