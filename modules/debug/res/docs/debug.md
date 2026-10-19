---
id: debug
group: tools
title: Debug Draw
order: 1
---
# Debug Draw

Lines, arrows, circles, rectangles, crosses and labels drawn over the game in Play and in `aego run`. They help you see what code does: a path, a sight cone, a velocity.

## Scripts

    import "aego/debug"

    func Update(dt float) {
      debug.Arrow(node.WorldPosition(), node.WorldPosition() + Velocity * 0.2, Color{0, 1, 0, 1})
      debug.Pen("ai.path", 2)
      debug.Cross(goal, 0.3, Color{1, 0, 1, 1})
    }

A primitive lasts one frame by default. `debug.Pen(channel, ttl)` sets the channel and the lifetime of what the script draws next. A positive ttl counts game seconds and stops while paused. A negative ttl lasts until `debug.Clear`. `PenUnscaled` counts real seconds. A one-frame primitive drawn just before a pause stays visible until the game resumes.

## Channels

Every primitive belongs to a channel. The Debug Draw panel lists the channels of the running game and turns them on and off, one by one or by preset (Collision, Velocity, AI, Camera). Physics shapes, contacts, AABBs, joints, islands and the broadphase tree are the `physics.*` channels; they are off by default and the Physics menu turns them on.

## Go

    if d, ok := sdk.Debug(ctx); ok {
      d.Circle(center, 2, &sdk.DebugStyle{Color: sdk.Color{R: 1, A: 1}, Channel: "mygame.spawn", Life: debugdraw.ForSeconds(1)})
    }

`Style.Fade` fades the alpha, or the alpha and the size, over the lifetime. `Thickness` draws up to four pixel-wide lines. `Space: Screen` takes positions in logical pixels of the game view.

## Limits

16384 primitives per world and 4096 per channel; more are dropped and counted in the panel. `debug { enabled=false }` in `project.aego` turns every call into a no-op.
