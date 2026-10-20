---
id: physics
try: try/Body2D.aescene
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

Aego simulates 2D rigid bodies with its own solver. A body is a node with **Rigid Body 2D**; colliders on that node and its children give it a shape; scripts react to contacts and query the world. Physics runs during Play and in `aego run`. In edit mode nothing moves, and the Scene View shows collider outlines and their problems.

Quick start: Create → Physics → Rigid Body gives a falling box, Create → Physics → Static Collider a floor for it. Press Play.

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
try: try/CircleCollider2D.aescene
try_select: physics.CircleCollider2D
icon: comp.rigidbody
parent: physics
order: 2
title: Components
---
# Components

| Component | Purpose |
|---|---|
| Rigid Body 2D | makes the node a body: `static`, `kinematic` or `dynamic` |
| Box / Circle / Capsule / Polygon / Edge Collider 2D | shapes |
| Distance Joint 2D | keeps a distance to the `Target` body, rigid or springy |
| Revolute Joint 2D | pins to the `Target` body at `Anchor`, with optional limit and motor |

Every collider has `Offset`, `IsTrigger`, `Friction`, `Restitution` and `Density`. Capsules lie along the node's local X axis; `Length` includes the caps. Polygons are convex with 3 to 8 points. Edge colliders are polylines for level geometry; `Loop` closes them, `OneSided` makes them solid only from the side the points wind towards (left-to-right points are solid from above).

Rigid Body 2D:

| Field | Meaning |
|---|---|
| `Type` | `static` never moves, `kinematic` moves by velocity and ignores forces, `dynamic` reacts to everything |
| `MassMode`, `Mass` | `auto` takes mass from collider density, `override` uses `Mass` |
| `GravityScale` | multiplier of the project gravity |
| `LinearDamping`, `AngularDamping` | slow the body down over time |
| `FixedRotation` | the body never turns |
| `Bullet` | swept collision for small fast bodies |
| `AllowSleep` | let the body rest when it stops |
| `Interpolate` | draw between physics steps; scripts still see the exact pose |
| `Layer`, `Mask` | see Layers |
| `Velocity`, `AngularVelocity`, `Awake` | live values during Play, read-only |

---
id: physics/hierarchy
icon: panel.hierarchy
parent: physics
order: 3
title: Bodies and hierarchy
---
# Bodies and hierarchy

A collider belongs to the nearest node at or above it that has a Rigid Body 2D. One body can therefore be built from several child nodes, each with its own collider; mass, centre of mass and inertia come from all of them. A collider with no body above it becomes a static body of its own.

The collider's position, rotation and uniform scale relative to its body are baked into the shape. Boxes and polygons also take non-uniform scale; circles and capsules need a uniform one and report PHY001 otherwise. Nodes under a 3D parent are not simulated (PHY005).

The transform of a moving body belongs to physics. Write it from `FixedUpdate` or through `Teleport` and it is applied as a teleport at the next step, keeping the velocity. A write from `Update` is reported once per body (PHY007) and applied at the next step. Static and kinematic bodies follow their node every step. Reparenting a moving body during Play recreates it with the same world pose and velocity (PHY006).

---
id: physics/layers
icon: view.overlays
parent: physics
order: 4
title: Layers
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

Body methods are called on the component:

    body := node.Get<Body2D>()
    v := body.Velocity
    body.SetVelocity(Vec2{x * Speed, v.Y})
    body.ApplyImpulse(JumpImpulse)

| Method | |
|---|---|
| `ApplyForce(f)`, `ApplyForceAt(f, p)`, `ApplyTorque(t)` | push during the next step, call from `FixedUpdate` |
| `ApplyImpulse(j)`, `ApplyImpulseAt(j, p)` | change the velocity at once, scaled by mass |
| `SetVelocity(v)`, `SetAngularVelocity(w)` | |
| `MoveTo(p, angle)` | drive a kinematic body to a pose by the end of the next step |
| `Teleport(p, angle)` | move without sweeping, velocity kept |
| `WakeUp()`, `Sleep()`, `Mass()` | |

Queries live in `aego/physics`. They ignore the caller's own body and skip triggers:

| Function | |
|---|---|
| `Raycast(origin, dir, maxDist) RayHit` | closest hit |
| `Linecast(from, to, mask) RayHit` | first hit on the layers in `mask`, `-1` for all |
| `RaycastAll(from, to, out) int` | fills `out` nearest first |
| `OverlapPoint(p, out)`, `OverlapCircle(c, r, out)`, `OverlapBox(c, size, out)` | bodies touching the area |
| `Gravity()`, `SetGravity(g)` | |
| `Layer(name)`, `LayerMask(name)` | layer index and mask bit by name |

Functions that fill `out` write at most `len(out)` entries; size the slice once with `append`. Queries work only during Play.

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
try: try/EdgeCollider2D.aescene
try_select: physics.EdgeCollider2D
icon: tool.move
parent: physics
order: 8
title: Editor tools
---
# Editor tools

![Collider outlines and contacts drawn over the Game View during Play](shots/physics-debug.png)

- **Handles.** Select a node with a collider to drag its sides, radius, length or vertices; double-click an edge of a polygon to add a vertex, Alt-click a vertex to remove it. Joints show anchors on both bodies. Every drag is one undo step.
- **Snap to Sprite** in the inspector of box, circle and capsule colliders fits the shape to the node's sprite.
- **Physics menu.** Debug Overlay with Shapes, Contacts, AABBs, Joints, Islands and Broadphase Tree. In edit mode only shapes are drawn.
- **Vectors** (V in the Scene View) draws velocity, angular velocity, forces, contact impulses, gravity and script fields with `@export(gizmo="vector")`. Drag the tip of a field arrow to change the value.
- **Game View → Overlays** draws the same layers over the running game with the game camera.
- **Problems** lists collider and joint errors with a link to the node; broken geometry blocks Play.
- **Profiler → Physics** shows bodies, pairs, islands and stage timings during Play.

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
| Reparent a moving body | recreated with world pose and velocity, PHY006 |
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

`modules/physics/world` is the solver under the components, usable on its own from Go (see `demos/physics2d`).

`Step(dt)` collides once and then solves in `Config.Substeps` substeps (4 by default): each substep applies gravity, solves contacts as stiff springs that push overlapping shapes apart at most `ContactPushVelocity` units per second, moves the bodies and relaxes the push so it does not turn into a bounce. The engine steps physics at the fixed rate (`FixedHz`, 60 by default), never with the frame delta.

A body moves at most `MaxTranslationPerStep` units and turns at most `MaxRotationPerStep` radians per step. Fast small bodies should set `Bullet`: their contacts look ahead by the distance they travel in one step.

Every definition is validated: NaN, infinities, negative sizes, concave polygons and zero-length segments are rejected with an error and the world stays unchanged.

| Material | Meaning | Mix of two shapes |
|---|---|---|
| `Friction` | 0 is ice, 1 is rubber | `sqrt(a·b)` |
| `Restitution` | 0 stops, 1 bounces back at full speed | `max(a, b)` |
| `Density` | mass per square unit | — |

Bounces slower than `RestitutionThreshold` are not restituted. A group of touching bodies falls asleep when all of them move slower than `SleepLinear` and `SleepAngular` for `SleepTime` seconds; a contact with an awake body, a force, an impulse, a teleport or a new joint wakes it.

The same binary on the same CPU with the same sequence of calls produces bit-identical results; `Snapshot()` and `Restore()` capture the whole state for replays and rollback.

---
id: physics/pitfalls
icon: status.warning
parent: physics
order: 11
title: Common mistakes
---
# Common mistakes

- **Moving a dynamic body with SetPosition from Update.** Use `SetVelocity`, forces or `Teleport` from `FixedUpdate`.
- **Reading input edges in FixedUpdate.** `input.Pressed` is true for one frame, and a frame may have zero or two fixed steps. Read it in `Update` and act in `FixedUpdate`, or use `input.Down`.
- **Circle on a stretched node.** Circles and capsules need a uniform scale. Scale the collider fields instead.
- **Objects falling through thin floors.** Mark small fast bodies as `Bullet`, or make the floor thicker.
- **Wobbling tall towers.** Use the stiff profile, stagger the boxes like bricks, or build the tower as one body.
- **Nothing collides.** Check both masks, the layer matrix, and that the node is 2D and not under a 3D parent.
- **Units that feel floaty.** A 64-pixel sprite at zoom 64 is one unit; gravity 9.81 assumes a unit is about a metre.
