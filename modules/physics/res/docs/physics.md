---
id: physics
try: BodyFalling.aescene
try_select: physics.Body2D
icon: cat.physics
group: physics
group_title: Physics 2D
group_icon: cat.physics
group_order: 50
title: Physics 2D
order: 1
---
# Physics 2D

Aego simulates 2D rigid bodies with its own solver. A body is a node with **Rigid Body 2D**; colliders on that node and its children give it a shape. Scripts react to contacts and query the world.

Physics runs during Play and in `aego run`. In edit mode nothing moves, and the Scene View shows collider outlines and their problems.

To try it:

1. **Create → Physics → Rigid Body** gives a falling box.
2. **Create → Physics → Static Collider** gives a floor for it.
3. Press Play.

---
id: physics/units
icon: view.grid
parent: physics
order: 1
title: Units and axes
---
# Units and axes

Physics works in world units. One unit is whatever size you choose for your game; shapes between 0.1 and 10 units behave best. `render.pixelsPerUnit` in `project.aego` is used only for the default size of imported sprites, the default `Camera2D.Zoom` and **Snap to Sprite**; physics never reads it, so changing it does not change how anything falls.

Axes follow the 2D scene: +X right, +Y down, positive angles turn clockwise. Default gravity is `(0, 9.81)`, pointing down the screen. Normals and velocities in events and queries use the same axes, so a floor under a body has a normal with a negative Y.

---
id: physics/components
try: CircleComponentsOriginal.aescene
try_select: physics.CircleCollider2D
icon: comp.rigidbody
parent: physics
order: 2
title: Components
---
# Components

These components make a node part of the simulation. Each has its own page in the reference with every field.

| Component | Purpose |
|---|---|
| [Rigid Body 2D](ref:physics.Body2D) | makes the node a body: `static`, `kinematic` or `dynamic` |
| [Box](ref:physics.BoxCollider2D), [Circle](ref:physics.CircleCollider2D), [Capsule](ref:physics.CapsuleCollider2D), [Polygon](ref:physics.PolygonCollider2D) and [Edge Collider 2D](ref:physics.EdgeCollider2D) | shapes |
| [Distance Joint 2D](ref:physics.DistanceJoint2D) | keeps a distance to the `Target` body, rigid or springy |
| [Revolute Joint 2D](ref:physics.RevoluteJoint2D) | pins to the `Target` body at `Anchor`, with an optional limit and motor |

Every collider has `Offset`, `IsTrigger`, `Friction`, `Restitution` and `Density`.

- Capsules lie along the node's local X axis. `Length` includes the caps.
- Polygons are convex, with 3 to 8 points.
- Edge colliders are polylines for level geometry. `Loop` closes them. `OneSided` makes them solid only from the side the points wind towards: points from left to right are solid from above.

A body's `Type` decides how it moves: `static` never moves, `kinematic` moves by its velocity and ignores forces, `dynamic` reacts to forces and contacts. Its velocity and whether it is awake are runtime state, listed on the [Rigid Body 2D](ref:physics.Body2D) page.

---
id: physics/hierarchy
icon: panel.hierarchy
parent: physics
order: 3
title: Bodies and hierarchy
---
# Bodies and hierarchy

A collider belongs to the nearest node at or above it that has a Rigid Body 2D. One body can therefore be built from several child nodes, each with its own collider; mass, centre of mass and inertia come from all of them. A collider with no body above it becomes a static body of its own.

The collider's position, rotation and uniform scale relative to its body are baked into the shape. Boxes and polygons also take non-uniform scale; circles and capsules need a uniform one and report [PHY001](ref:PHY001) otherwise. Nodes under a 3D parent are not simulated ([PHY005](ref:PHY005)).

The transform of a moving body belongs to physics. Write it from `FixedUpdate` or through `Teleport` and it is applied as a teleport at the next step, keeping the velocity. A write from `Update` is reported once per body ([PHY007](ref:PHY007)) and applied at the next step. Static and kinematic bodies follow their node every step. Reparenting a moving body during Play recreates it with the same world pose and velocity ([PHY006](ref:PHY006)).

---
id: physics/layers
icon: view.overlays
parent: physics
order: 4
title: Collision layers
---
# Layers

Up to 32 named layers and the pairs that collide are declared in `project.aego` or in Project Settings → Physics:

    physics {
      layers=[Default, Ground, Player, Enemy]
      collide {
        Player=[Ground, Enemy]
        Enemy=[Ground, Player]
      }
    }

The matrix is symmetric: `Player=[Enemy]` also lets enemies hit players. Without a `collide` block every layer collides with every other. An unknown layer name in `collide` is a load error.

A body stores its layer as an index, so renaming a layer keeps every body on it; clearing a name keeps the index. Two shapes collide when both bodies' `Mask` accept the other's layer and the matrix allows the pair. Changing a layer or mask during Play ends the contacts it forbids in the next step.

---
id: physics/events
icon: panel.events
parent: physics
order: 5
title: Events
---
# Events

Declare a method with one of these names in any script on the body node or on the collider node:

    func OnCollisionEnter(other NodeRef, c physics.Contact)
    func OnCollisionStay(other NodeRef, c physics.Contact)
    func OnCollisionExit(other NodeRef, c physics.Contact)
    func OnTriggerEnter(other NodeRef)
    func OnTriggerExit(other NodeRef)

Both participants get the event, each from its own side. `other` is the other body's node; `c.Normal` points from the other body into the receiver, so a floor under you reports `c.Normal.Y < 0`. `c.SelfCollider` and `c.OtherCollider` name the collider nodes. Events run after the physics step, before `script.events_fixed` ends the fixed tick.

`OnCollisionStay` runs every step while two bodies touch, and only when some script handles it; prefer Enter/Exit counters. Exit also comes when a body is destroyed or disabled; `other.IsAlive()` is then false.

Destroying nodes inside a handler is safe: the removal happens after all handlers ran.

---
id: physics/scripting
icon: file.script
parent: physics
order: 6
title: Scripting
---
# Scripting

Scripts move bodies through the Rigid Body 2D component and ask the world what is where through `aego/physics`.

```aes
body := node.Get<Body2D>()
v := body.Velocity
body.SetVelocity(Vec2{x * Speed, v.Y})
body.ApplyImpulse(JumpImpulse)
```

## Moving a body

- Forces and torque act during the next physics step. Call `ApplyForce` and `ApplyTorque` from `FixedUpdate`.
- Impulses change the velocity at once, scaled by mass. They suit jumps and hits.
- `MoveTo` drives a kinematic body to a pose by the end of the next step. `Teleport` moves a body without sweeping and keeps its velocity.

Every method is listed on the [Rigid Body 2D](ref:physics.Body2D) page.

## Queries

Raycasts, line casts and overlap tests ignore the caller's own body and skip triggers. They work only during Play. Functions that fill an `out` slice write at most `len(out)` entries, so size the slice once with `append` and reuse it.

Every function is listed on the [aego/physics](ref:aego/physics) page.

---
id: physics/settings
icon: panel.settings
parent: physics
order: 7
title: Settings
---
# Settings

![Project Settings → Physics with the gravity and contact previews](shots/settings-physics.png)

Project Settings → Physics edits the `physics` block of `project.aego`: gravity, substeps, iterations, contact stiffness, sleep thresholds, layers and the matrix. An empty or zero field takes the engine default, shown greyed out. Substeps and both iteration counts accept 1 to 64. Changes apply at the next Play.

Tall free-standing stacks and fast motors need the stiff profile: `substeps=8` and `contactHertz=60`, at about twice the solver cost. At the default 30 Hz with 4 substeps a free column stands up to about twelve boxes; bonded brick layers stand much higher.

---
id: physics/editor
try: EdgeColliderRampOriginal.aescene
try_select: physics.EdgeCollider2D
icon: tool.move
parent: physics
order: 8
title: Editor tools
---
# Editor tools

The editor draws colliders, contacts and forces and lets you shape colliders with handles. These tools work in the Scene View, and some of them over the running game.

![Collider outlines and contacts drawn over the Game View during Play](shots/physics-debug.png)

## Handles

Select a node with a collider to drag its sides, radius, length or vertices. Double-click an edge of a polygon to add a vertex; Alt-click a vertex to remove it. Joints show anchors on both bodies. Every drag is one Undo step.

**Snap to Sprite** in the Inspector of box, circle and capsule colliders fits the shape to the node's sprite.

## Overlays

- The **Physics** menu turns on the debug overlay: shapes, contacts, bounding boxes, joints, islands and the broadphase tree. In edit mode only shapes are drawn.
- **Vectors** (`V` in the Scene View) draws velocity, angular velocity, forces, contact impulses, gravity and script fields with `@export(gizmo="vector")`. Drag the tip of a field arrow to change the value.
- **Game View → Overlays** draws the same layers over the running game, with the game camera.

## Problems and profiling

- **Problems** lists collider and joint errors with a link to the node. Broken geometry blocks Play.
- **Profiler → Physics** shows bodies, pairs, islands and the time of each stage during Play.

---
id: physics/lifecycle
icon: panel.history
parent: physics
order: 9
title: Lifecycle
---
# Lifecycle

What happens to bodies, shapes and contact events when the scene changes during Play.

| Action during Play | Result |
|---|---|
| Destroy a body node | its shapes leave the world at the next step, Exit events in the same step |
| Destroy a collider child | the shape is removed, mass and inertia recomputed |
| Disable a node | all shapes below it switch off with Exit events; enabling brings Enter at the next step |
| Remove Rigid Body 2D | child colliders become static bodies |
| Remove a collider | the shape is removed with Exit |
| Change layer or mask | forbidden pairs end in the same step |
| Change a trigger flag or the geometry | contacts of the shape end, new ones begin at the next step |
| Add a body or collider | picked up at the next step, starting at rest |
| Reparent a moving body | recreated with world pose and velocity, [PHY006](ref:PHY006) |
| Script hot reload | the world keeps running, handlers are rebound by name |
| Native rebuild | the play world is recreated with the editor |
| Stop | the world is destroyed without Exit events |

---
id: physics/world
icon: vec.gravity
parent: physics
order: 10
title: World
---
# World

The physics world steps all bodies together at the fixed rate, 60 times a second by default, never with the frame time. This page describes how it moves bodies and what the material values mean.

## Steps

Each step finds contacts once, then solves in substeps, 4 by default. A substep applies gravity, pushes overlapping shapes apart, moves the bodies and settles the push so it does not turn into a bounce. More substeps give stiffer contacts; see [Settings](ref:physics/settings).

A body moves at most a set distance and angle per step. Small fast bodies should set `Bullet`: their contacts look ahead by the distance they travel in one step.

Every body and shape is checked when it is made. Invalid numbers, negative sizes, concave polygons and zero-length segments are rejected with an error, and the world stays as it was.

## Materials

| Material | Meaning | When two shapes touch |
|---|---|---|
| `Friction` | 0 is ice, 1 is rubber | `sqrt(a·b)` |
| `Restitution` | 0 stops, 1 bounces back at full speed | `max(a, b)` |
| `Density` | mass per square unit | — |

Very slow bounces are not restituted, so resting bodies stay at rest.

## Sleep

A group of touching bodies falls asleep when all of them have moved slowly for a while. A contact with an awake body, a force, an impulse, a teleport or a new joint wakes the group.

## Determinism

The same game on the same CPU with the same input gives the same result every time. Rewinding Play in the editor relies on this.

---
id: physics/pitfalls
icon: status.warning
parent: physics
order: 11
title: Common mistakes
---
# Common mistakes

These are the mistakes that most often make physics feel wrong. Each one names what you see and what to change.

## A dynamic body jitters when moved

Moving a dynamic body with `SetPosition` from `Update` fights the solver. Use `SetVelocity`, forces or `Teleport` from `FixedUpdate`.

## A jump is sometimes missed

Read action-based `input.Pressed("Jump")` directly in `FixedUpdate`. A press is queued until the first fixed step, even when the rendering frame has no fixed steps, and is not repeated by subsequent steps. `input.Down` means held, not newly pressed: it is not a substitute for a jump edge. If another input context consumes the action, check that context and its bindings. See [Input: frames and fixed steps](ref:input).

## A circle collider reports a problem

Circles and capsules need a uniform scale. Change the collider's fields instead of stretching the node.

## Objects fall through thin floors

Mark small fast bodies as `Bullet`, or make the floor thicker.

## Tall towers wobble

Use the stiff profile in [Settings](ref:physics/settings), stagger the boxes like bricks, or build the tower as one body.

## Nothing collides

Check both bodies' masks and the layer matrix, and that the node is 2D and not under a 3D parent.

## Motion feels floaty

Gravity 9.81 assumes a unit is about a metre. A 64-pixel sprite at zoom 64 is one unit.
