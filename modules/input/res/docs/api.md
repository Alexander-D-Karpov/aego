---
id: aego/input
summary: Reads actions, raw keys, the mouse and gamepads, manages input contexts and lets players rebind controls.
---

Prefer actions (`Pressed("Jump")`, `Axis2("Move")`) to raw keys: actions are bound in the project, can be rebound by players and work with every device. The `P` variants read one player. The documentation sandbox has no devices, so the example reads zero values and reports that no device was used; in a game the same code moves the node. See [Input](ref:input).

## Choose an operation

| Task | Start with |
|---|---|
| React to an action | Pressed for an edge, Down for holding, Axis/Axis2 for movement |
| Read one player | The matching `P` variant and the player index |
| Control overlapping UI/game bindings | Input contexts and action consumption |
| Offer rebinding | Rebind operations and glyphs, not hard-coded keyboard labels |
| Inspect a device | ActiveDevice and low-level key, mouse or gamepad operations |

Action-based Pressed is delivered to the first fixed step after a press, including a press on a render frame with no fixed step. Subsequent fixed steps do not repeat that edge. Check input contexts if another consumer handles it first. Raw KeyPressed describes a frame edge; do not assume that every low-level operation has the action queue's contract.

The Run button below demonstrates script execution and zero-device results only. To test movement with real input, follow the [Input guide](ref:input) in a desktop project. Resetting a script playground is not a reset of a physics world.

## Example

```aes run
package demo

import (
    "aego/input"
    "aego/log"
)

script Player {
    @export var Speed float = 4

    var jumps int
    var frames int

    func Update(dt float) {
        move := input.Axis2("Move")
        node.SetPosition(node.Position() + move * Speed * dt)
        if input.Pressed("Jump") {
            jumps++
        }
        if input.Down(input.KeyF1) || input.MousePressed(input.MouseLeft) {
            log.Info("cursor at " + string(input.MouseWorld().X) + ", " + string(input.MouseWorld().Y))
        }
        frames++
        if frames % 60 == 0 {
            device := input.ActiveDevice()
            if device < 0 {
                log.Info("no input yet; move " + string(move.X) + ", " + string(move.Y) + ", jumps " + string(jumps))
            } else {
                log.Info("device " + string(device) + ", jump is " + input.Glyph("Jump"))
            }
        }
    }
}
```
