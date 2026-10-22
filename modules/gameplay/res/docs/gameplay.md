---
id: gameplay
try: try/Motion2D.aescene
try_select: gameplay.Motion2D
icon: gameplay.motion
group: gameplay
title: Character motion
order: 2
---
# Character motion

[Motion 2D](ref:gameplay.Motion2D) moves a character with a kinematic [Rigid Body 2D](ref:physics.Body2D). Each fixed step it casts the body's colliders along the move, slides along what it hits and moves the body to the result, so the character never sinks into walls and still pushes crates.

Quick start: give the player node a Rigid Body 2D with Type kinematic, a collider and Motion 2D. From a script call `Move` with the horizontal input and `Jump` when the jump button goes down:

```aes
m := node.Get<Motion2D>()
m.Move(input.Axis("Move"))
if input.Pressed("Jump") {
    m.Jump()
}
if input.Released("Jump") {
    m.JumpReleased()
}
```

`Move` stays until the next call; `Jump` is one request that the controller keeps for Jump Buffer seconds, so a jump pressed just before landing still happens.

| Mode | Behaviour |
|---|---|
| platformer | walks on floors under gravity, jumps, climbs steps and slopes |
| topDown | moves freely in the plane with `MoveDir`, slides along walls |

Floors, walls and ceilings are told apart by the angle of the surface to Up Direction: a surface steeper than Max Slope is a wall. On a floor the character follows its slope without sliding down, and Snap To Ground keeps it on the floor when walking down a slope or a small step. Steps up to Step Height are climbed without jumping.

The character rides on the body it stands on when Moving Platforms is on; Platform On Leave decides what happens to the platform's speed after a jump. With Push Bodies on, dynamic bodies do not stop the character, it pushes them.

Coyote Time lets a jump work a moment after walking off a ledge, and Jump Cut Multiplier shortens a jump when the button is released early. `Dash`, `AddVelocity`, `Stop` and `Teleport` cover dashes, knockback and respawns.

Signals `Landed`, `LeftGround`, `Jumped`, `HitWall` and `HitCeiling` arrive in the same fixed step as collisions. The Runtime fields show the velocity, whether the character is grounded or touching a wall, and the floor it stands on.

**Problems** lists a platformer with Gravity Scale 0 and no Jump Speed (GMP002). A Motion 2D on a body that is not kinematic cannot move it; the console says so once (GMP001).
