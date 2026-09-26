// Package scene implements the runtime node graph: hierarchy, transforms and
// components.
//
// Structural changes are deferred. Destroy, Remove and Reparent take effect
// logically at once (IsAlive, Has and Get report the new state immediately)
// but are applied to storage at Flush. Add is the mirror image: the data is
// written to a pending buffer and visible through Get straight away, yet it
// only enters iteration — Each, Data, Query — after Flush. A system that adds
// components while walking a store therefore cannot loop over its own output.
//
// Pointers returned by Add and Get are valid until the end of the current
// phase. Keep a NodeHandle instead.
package scene
