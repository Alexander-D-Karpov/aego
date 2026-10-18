---
id: input
title: Input
order: 15
---
# Input

Games read **actions**, not keys. An action has a name, a type and bindings; the same script works with a keyboard and a gamepad.

## Actions

    input version=2 {
      stack=[Gameplay]
      action {
        name=Move
        type=axis1
        bind=["Key.A/Key.D", "Key.Left/Key.Right", "Pad.LeftX"]
      }
      action {
        name=Jump
        type=button
        buffer=0.1
        bind=["Key.Space", "Pad.A"]
      }
      context {
        name=Gameplay
        actions=[Move, Jump]
      }
    }

Types: `button`, `axis1`, `axis2`, `pointer`, `pointerDelta`.

A binding is a control or a composite:

| Form | Example |
|---|---|
| one control | `Key.Space`, `Mouse.Left`, `Pad.A`, `Pad.LT` |
| negative/positive | `Key.A/Key.D` |
| up/down/left/right | `Key.W/Key.S/Key.A/Key.D` |
| stick | `Pad.LeftStick` |
| chord | `Key.Z+Key.S` |

Options follow the control: `invert`, `scale=2`, `deadzone=0.3`.

- Pad buttons: `A B X Y Back Guide Start L3 R3 LB RB DPadUp DPadDown DPadLeft DPadRight`.
- Pad axes: `LeftX LeftY RightX RightY LT RT`.
- `axis2` points Y down, like the 2D world.
- A stick uses a radial dead zone and a single axis an axial one. The default is `deadzone=0.2`.

`interaction` changes when a button fires: `press` (default), `release`, `hold`, `tap`, `doubleTap`, `toggle`. `time` sets the window in seconds.

## Contexts

A context is a named set of actions. The stack decides what is read: higher `priority` first, then the last pushed.

- A context with `consume=true` hides its controls from the contexts below it: a menu with `Back` on Escape stops `Pause` on Escape.
- Switching contexts sends no fake Pressed or Released. `release=true` on a context sends Released for its held actions when it leaves the stack.

## Frames and fixed steps

`Pressed` in `Update` is true for one frame. In `FixedUpdate` it is true in the first fixed step after the press.

- A tap between two fixed steps still gives exactly one `Pressed`, followed by one `Released`.
- `Held` is true in every step of a frame.
- Losing window focus or a gamepad releases every held action in the same step. `input.DeviceChanged()` is true for that frame.

## Scripts

    import "aego/input"

    func FixedUpdate(dt float) {
      x := input.Axis("Move")
      if grounded && input.Buffered("Jump", 0) {
        input.Consume("Jump")
        jump()
      }
    }

| Function | |
|---|---|
| `Pressed Released Held(action) bool` | action state |
| `Axis(action) float` | axis value |
| `Axis2(action) Vec2` | 2D action value |
| `Buffered(action, window) bool`, `Consume(action)` | press buffer |
| `PushContext PopContext SetContext` | context stack |
| `PressedP ReleasedP HeldP AxisP Axis2P(player, action)` | per player |
| `ActiveDevice() int`, `DeviceChanged() bool`, `Glyph(action) string` | devices and hints |
| `BeginRebind(action, slot) bool`, `Rebinding()`, `RebindConflict()`, `ResetBinding`, `ResetBindings`, `SaveBindings` | menus |

Rebinds and look options are player settings: they are saved in the player profile; an old `user://input.aetext` is imported once.

Raw keys (`Down`, `KeyPressed`, `KeyReleased`, `MouseDown`, `MousePressed`, `MousePos`, `MouseWorld`) stay for prototypes. Do not ship with them.

## Players

`players=2` gives each player their own action state.

- Keyboard and mouse belong to player 0.
- A gamepad goes to the first player without a device on its first press.
- In Go, `Assign(player, device)` fixes the mapping.

## Recording and tests

    s := sdk.Actions(world)
    s.Inject(40, "Jump", 1)
    s.Inject(43, "Jump", 0)
    s.Record(input.RecordMeta{Project: "mygame", FixedHz: 60})
    data := s.StopRecording()
    s.Replay(data)

A replay replaces every device. With the same scene, seed, physics settings and fixed step, a replay reproduces the run bit for bit.

## Actions from modules

A module adds its own actions and contexts without importing the input module:

    sdk.ProvideInputActions(r, sdk.InputActionSet{
      Actions:  []sdk.InputAction{{Name: "Kick", Type: "button", Bindings: []string{"Key.K", "Pad.Y"}}},
      Contexts: []sdk.InputContext{{Name: "Combat", Actions: []string{"Kick"}, Default: true}},
    })

How module actions combine with the project:

- A project action or context with the same name wins.
- A context marked `Default` joins the start stack.
- Module actions do not appear on the settings page; the project file shows only what it declares.

## Editor

- **Project Settings → Input** edits actions, bindings, contexts and the start stack. To capture a control, click it, then press a key, a mouse button (not the left one), a pad button or move a stick; Escape cancels. Controls bound to two actions of one context are red, and the tooltip names the other action.
- **Input Devices** lists keyboard, mouse and gamepads, shows live action values during Play, and has a virtual gamepad: buttons and two sticks you drag with the mouse.
- **Input Focus** shows the context stack and, for every action, the context it was read through and the consuming context that blocked it.

In the editor, Escape goes to the game, so pause menus work in Play. **Shift+Esc** or a click outside the Game View returns input to the editor; the binding is the command Release Game Input and can be changed in the keymap.

`Inject(0, action, v)` applies on the next frame. Use it while game time is paused or scaled to 0, when fixed steps do not advance.
