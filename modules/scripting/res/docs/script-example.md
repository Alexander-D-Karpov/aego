---
id: language/script-example
parent: language/scripts-and-lifecycle
order: 1
title: Animated script example
try: ScriptGuideOriginal.aescene
try_select: engine.Script
---
# Animated script example

The original spinner uses `Ready` and `Update` to rotate and bob. Its exported `Speed`, `Bob` and `Rate` fields can be edited in the Inspector.

Run a smaller example that moves its own node when Play starts:

```aes run
package demo

script MoveOnReady {
    func Ready() {
        node.SetPosition(Vec2{2, 0})
    }
}
```
