---
id: gameplay
try: MotionGuideOriginal.aescene
try_select: gameplay.Motion2D
icon: cat.gameplay
group: gameplay
title: Gameplay kit
order: 2
---
# Gameplay kit

The gameplay module adds the parts most 2D games rebuild by hand: a character controller, hit points, hitboxes and hurtboxes, kill zones, checkpoints with respawn, pickups and timed removal. They are components, so they are set up in the Inspector, saved with the scene and driven from scripts with a few calls.

Quick start: Create → Gameplay → Platformer Character gives a node with a kinematic Rigid Body 2D, a collider, Motion 2D, Health, Hurtbox 2D and Respawn. Give it a script that calls `Move` and `Jump`, add a floor and press Play.

In the interactive guide, walk with A/D or the arrow keys and jump with Space, W or Up.

Two complete games use the kit: `aego new mygame --example platformer` (stomping enemies with a foot hitbox, a pit with a kill zone, a checkpoint, a persisted respawn point and an Animator for the player's look, with a slash on a hit frame) and `aego new mygame --example topdown` (a sword swung towards the stick, the mouse or the walking direction, enemies whose State Machine switches between Patrol and Chase while their Animator shows it, a lunge by root motion, coins and lava). The launcher offers both under **Examples**.

| Component | Purpose |
|---|---|
| Motion 2D | walks, jumps and slides a kinematic body: slopes, steps, moving platforms, top-down movement |
| Health | hit points, team, invulnerability after a hit, death and revive |
| Hurtbox 2D | area where hitboxes hurt a Health |
| Hitbox 2D | area that damages hurtboxes, with knockback |
| Lifetime | removes a node after some seconds |
| Kill Zone 2D | kills or respawns what falls into it |
| Checkpoint 2D | moves the respawn point |
| Respawn | brings a node back to its respawn point |
| Pickup 2D | fires `Collected` when a character touches it |
| State Machine | named states with Enter, Exit, Fixed and Update hooks in the node's scripts |

Everything runs during Play and in `aego run`, in the fixed step, and allocates nothing per step. Hurtboxes, hitboxes and zones are areas in their node's space, not physics colliders: they never push anything and need no collision layers.

---
id: gameplay/motion
try: MotionGuideOriginal.aescene
try_select: gameplay.Motion2D
icon: gameplay.motion
parent: gameplay
order: 1
title: Character motion
---
# Character motion

[Motion 2D](ref:gameplay.Motion2D) moves a character with a kinematic [Rigid Body 2D](ref:physics.Body2D). Each fixed step it casts the body's colliders along the move, slides along what it hits and moves the body to the result, so the character never sinks into walls and still pushes crates.

Try walking with A/D or the arrow keys and jumping with Space, W or Up; the lift, slope and walls use the real physics system.

    m := node.Get<Motion2D>()
    m.Move(input.Axis("Move"))
    if input.Pressed("Jump") {
        m.Jump()
    }
    if input.Released("Jump") {
        m.JumpReleased()
    }

`Move` stays until the next call; `Jump` is one request that the controller keeps for Jump Buffer seconds, so a jump pressed shortly before landing still happens.

| Mode | Behaviour |
|---|---|
| platformer | walks on floors under gravity, jumps, climbs steps and slopes |
| topDown | moves freely in the plane with `MoveDir`, slides along walls; Face Movement turns the node |

![Motion 2D during Play: the Runtime fields show velocity, Grounded and the contacts](shots/motion-play.png)

## Floors, slopes and steps

Floors, walls and ceilings are told apart by the angle of the surface to Up Direction: a surface steeper than Max Slope is a wall. On a floor the character follows its slope without sliding down, and Snap To Ground keeps it on the floor when walking down a slope or a small step. Steps up to Step Height are climbed without jumping. Safe Margin is the gap kept to every surface so the next cast never starts inside one.

In the air a wall stops the movement into it and never lifts the character: a capsule or circle that meets the corner of a ledge slides off it instead of hanging there.

## Ledges

With **Ledge Climb** above 0, a character in the air that moves into a wall whose top is at most Ledge Climb above its feet climbs onto it: it rises to the top, steps onto the ledge and lands there, over **Climb Time** seconds. Higher walls are left alone. The climb runs on fixed steps like the rest of the motion: input, gravity and root motion are ignored during it, and the velocity stays at zero. Knockback (`AddVelocity`), `Stop`, `Dash` and `Teleport` end it at once.

**Climbing** is true during the climb and **Climbed** fires when it starts. To animate it, pass the flag to the Animator in `FixedUpdate`, as the player of the platformer example does:

```aes
a := node.Get<Animator>()
a.SetBool("Climbing", m.Climbing)
```

and give the controller a `Climb` state entered from `any` while `Climbing` is true.

## Jumps

Set **Jump Height** and the controller works out the jump speed from it and gravity; **Jump Speed** sets the speed directly instead. Releasing the button while rising cuts the jump short by **Jump Cut Multiplier**, so a tap gives a small hop.

Two windows make jumping forgiving. **Coyote Time** lets a jump work for a moment after walking off a ledge. **Jump Buffer** keeps a jump pressed in the air until the character lands. **Air Control** decides how much of the ground acceleration is available in the air.

Every field is listed on the [Motion 2D](ref:gameplay.Motion2D) page.

## Platforms and bodies

With Moving Platforms the character rides on the body it stands on; Platform On Leave decides what happens to the platform's speed after a jump: `keep` adds it, `drop` loses it, `add` keeps only its horizontal part. With Push Bodies dynamic bodies do not stop the character, it pushes them.

`Dash(velocity, seconds)`, `AddVelocity` for knockback, `AddDisplacement(offset)` for root motion and scripted nudges, `Stop` and `Teleport` cover the rest. A displacement moves the character through walls and slopes like a move, on the next step, and leaves the velocity alone. The Runtime fields show the velocity, whether the character is grounded or touching a wall or ceiling, the floor normal and angle and the body it stands on.

---
id: gameplay/health
try: try/Health.aescene
try_select: gameplay.Health
icon: gameplay.health
parent: gameplay
order: 2
title: Health and damage
---
# Health and damage

[Health](ref:gameplay.Health) holds hit points. Hitboxes, kill zones and scripts damage it with `Damage(amount, source)`, `Kill(source)` and `Heal(amount)`; at zero it dies once and stays dead until `Revive(hp)`.

    hp := node.Get<Health>()
    if hp.Damage(2, other) {
        audio.PlayEvent(HitSound)
    }

After a hit the node ignores damage for Invuln Time seconds of game time. That time starts after the step, so all hits of one step count: two swords landing together both hurt, and `Died` still fires once. `SetInvulnerable(seconds)` makes the node untouchable on purpose.

At death a node with [Respawn](ref:gameplay.Respawn) comes back; otherwise Destroy On Death removes it and Despawn On Death returns it to its pool. Team keeps hitboxes of the same team away from it unless they allow friendly fire.

In the scene press J next to the dummy to strike it; it shows its hit points and comes back a second after it dies.

---
id: gameplay/hurtbox
try: HurtboxGuideOriginal.aescene
try_select: gameplay.Hurtbox2D
icon: gameplay.hurtbox
parent: gameplay
order: 3
title: Hurtboxes
---
# Hurtboxes

A [Hurtbox 2D](ref:gameplay.Hurtbox2D) is an area in its node's space where hitboxes hurt a Health: the one in its Health field, or the nearest Health on the node or above it. Several hurtboxes can share one Health, so a character can have a head, a body and a shield on child nodes.

Each hurtbox has an area in its node's space, a rectangle or the circle inside it. **Multiplier** scales the damage taken there: 2 for a weak spot, 0.5 for armour. Turn **Active** off to ignore hits, for dodges and invulnerable frames of an animation.

The dummy has a head hurtbox with Multiplier 2: jump and strike to hit it.

---
id: gameplay/hitbox
try: HitboxGuideOriginal.aescene
try_select: gameplay.Hitbox2D
icon: gameplay.hitbox
parent: gameplay
order: 4
title: Hitboxes
---
# Hitboxes

A [Hitbox 2D](ref:gameplay.Hitbox2D) damages the hurtboxes inside its area after the physics step. Policy decides how often one target is hit:

| Policy | Hits |
|---|---|
| onEnter | every time the target enters |
| oncePerActivation | once per target until `Activate` |
| periodic | every Period seconds while they overlap |

Team and Friendly Fire keep attacks off their own side; Team -1 takes the team of the nearest Health above, so a sword on a child node fights for its owner and never hurts it. Knockback is a velocity for a target with Motion 2D and an impulse for a dynamic body; it turns with the hitbox node, so a flipped character pushes the other way. Knockback From Center pushes away from the hitbox instead.

A sword swing is one call:

    if input.Pressed("Attack") {
        Sword.Get<Hitbox2D>().Pulse()
    }

`Pulse()` deals one round of damage in the next step, active or not; `Activate()` and `Deactivate()` switch a hitbox for the length of an attack. `Hit(target, damage)` fires for every target damaged.

Walk over the three spikes in the scene to compare the policies.


![A hitbox selected in the Scene View: area handles and the knockback arrow](shots/hitbox-gizmo.png)

---
id: gameplay/lifetime
try: LifetimePoolOriginal.aescene
try_select: gameplay.Lifetime
icon: gameplay.lifetime
parent: gameplay
order: 5
title: Lifetime
---
# Lifetime

[Lifetime](ref:gameplay.Lifetime) removes its node after Seconds of game time, or real time with Unscaled. With Despawn a spawned node goes back to its pool, which suits bullets, sparks and debris; otherwise the node is destroyed. Remaining shows the seconds left.

The balls in the scene are spawned from a prefab with `scene.Spawn`, return to the pool after two seconds and are spawned again from it; the bar on the left is destroyed after twenty.

---
id: gameplay/killzone
try: KillZoneGuideOriginal.aescene
try_select: gameplay.KillZone2D
icon: gameplay.killzone
parent: gameplay
order: 6
title: Kill zones
---
# Kill zones

A [Kill Zone 2D](ref:gameplay.KillZone2D) kills a Health whose node position enters the area, or triggers Respawn on a node without Health. Tag limits it to tagged nodes; without a tag every node with Health or Respawn counts. Put one under pits and a wide one below the level.

Walk into the pit in the scene: the character dies and comes back at the start.

---
id: gameplay/checkpoint
try: CheckpointGuideOriginal.aescene
try_select: gameplay.Checkpoint2D
icon: gameplay.checkpoint
parent: gameplay
order: 7
title: Checkpoints
---
# Checkpoints

A [Checkpoint 2D](ref:gameplay.Checkpoint2D) moves the respawn point of a node with Respawn that enters it, to its own position or to the node in Spawn Point. A checkpoint never replaces one with a higher Priority, so walking back does not lose progress; Once makes it work for the first node only. `Reached(by)` fires when a node takes it.

---
id: gameplay/respawn
try: RespawnGuideOriginal.aescene
try_select: gameplay.Respawn
icon: gameplay.respawn
parent: gameplay
order: 8
title: Respawn
---
# Respawn

[Respawn](ref:gameplay.Respawn) brings its node back after death, after a kill zone, or when a script calls `Trigger()`. The first respawn point is where the node starts; checkpoints and `SetPoint(position)` move it. After Delay seconds the node is teleported there, Restore Health revives it with full hit points, and `Respawned` fires.

With Persist the respawn point is kept in save games, so a loaded game restarts at the last checkpoint; Health keeps its hit points in save games too.

Press R in the scene to respawn on purpose.

---
id: gameplay/pickup
try: PickupGuideOriginal.aescene
try_select: gameplay.Pickup2D
icon: gameplay.pickup
parent: gameplay
order: 9
title: Pickups
---
# Pickups

A [Pickup 2D](ref:gameplay.Pickup2D) fires `Collected(by)` when a node enters it: a tagged node, or any node with Motion 2D. With Once it then disappears, back to its pool with Despawn.

    func Ready() {
        conn = Coin.Get<Pickup2D>().Collected.Connect(OnCoin)
    }

    func OnCoin(by NodeRef) {
        coins++
    }

The counter in the corner of the scene connects to every coin.

---
id: gameplay/statemachine
try: StateMachineGuideOriginal.aescene
try_select: gameplay.StateMachine
icon: gameplay.fsm
parent: gameplay
order: 10
title: State machines
---
# State machines

A [State Machine](ref:gameplay.StateMachine) lists state names; the scripts on its node hold the behaviour. For a state `Run` it calls, when they exist:

| Hook | When |
|---|---|
| `EnterRun()` | the machine enters Run |
| `ExitRun()` | the machine leaves Run |
| `FixedRun(dt float)` | every fixed step while in Run, before Motion 2D moves |
| `UpdateRun(dt float)` | every frame while in Run, after the scripts' Update |

    func UpdateIdle(dt float) {
        if node.Get<Motion2D>().Velocity.X != 0 {
            node.Get<StateMachine>().Go("Run")
        }
    }

    func EnterRun() {
        node.Get<SpriteRenderer>().Tint = RunColor
    }

`Go(state)` only queues the change: after the running hook returns, Exit of the old state and Enter of the new one run, then `Changed(from, to)` fires with the state indices. A `Go` inside Enter or Exit runs after that transition, so `EnterRun` may send the machine straight on to `Air`; at most eight changes happen in a row. `Is(state)` and `TimeIn()` read the state; Current, Previous and Time show it in the Inspector.

The machine enters Initial in the first frame, after the scripts' Ready. Hooks are found by name and found again after a hot reload, so renaming a hook takes effect at once. Lifecycle methods, event handlers and coroutines are never hooks. Problems warns about a machine without states, a state listed twice or an Initial that is not one of the states ([GMP005](ref:GMP005)).

Walk, stop and jump in the scene: the colour and the label follow the state.

---
id: gameplay/time
icon: play.speed
parent: gameplay
order: 11
title: Time and replays
---
# Time and replays

Motion, hits, zones and respawns run in the fixed step: `gameplay.fsm_fixed` and then `gameplay.motion` before physics, after `script.fixed`, and `gameplay.post` after physics, before scripts receive collision events. `gameplay.fsm_update` runs the Update hooks of state machines after `script.update`. Signals of one step therefore reach scripts in the same step as the collisions that caused them.

Damage dealt by a script in Update is applied at once; its `Damaged`, `Healed` and `Died` arrive at the end of the same frame from `gameplay.frame`, which also counts Lifetime. Invulnerability, respawn delays and periodic hits count game time and stop while the game is paused; Lifetime with Unscaled counts real time.

All component state, including the hidden state of the controller, hitboxes, zones and state machines, travels with the scene: rewind, replays and pools restore it exactly, and the state hash used by `aego test --determinism` includes it.

---
id: gameplay/scripting
icon: file.script
parent: gameplay
order: 12
title: Scripting
---
# Scripting

The components are reached with `node.Get<T>()`; there is no module to import.

| Component | Methods | Signals |
|---|---|---|
| Motion2D | `Move(x)`, `MoveDir(dir)`, `Jump()`, `JumpReleased()`, `Dash(velocity, seconds)`, `AddVelocity(v)`, `AddDisplacement(offset)`, `Stop()`, `Teleport(position)` | `Landed(speed)`, `LeftGround()`, `Jumped()`, `HitWall(normal)`, `HitCeiling()` |
| Health | `Damage(amount, source) bool`, `Heal(amount)`, `Kill(source)`, `Revive(hp)`, `SetInvulnerable(seconds)` | `Damaged(amount, source)`, `Healed(amount)`, `Died(source)`, `Revived()` |
| Hitbox2D | `Activate()`, `Deactivate()`, `Pulse()` | `Hit(target, damage)` |
| Checkpoint2D | | `Reached(by)` |
| Respawn | `Trigger()`, `SetPoint(position)` | `Respawned()` |
| Pickup2D | | `Collected(by)` |
| StateMachine | `Go(state) bool`, `Is(state) bool`, `TimeIn() float` | `Changed(from, to)` |

A landing shake and a death screen:

    func Ready() {
        landed = node.Get<Motion2D>().Landed.Connect(OnLanded)
        died = node.Get<Health>().Died.Connect(OnDied)
    }

    func OnLanded(speed float) {
        if speed > 8 {
            camera.Shake(0.3)
        }
    }

    func OnDied(source NodeRef) {
        GameOver.SetEnabled(true)
    }

A short name like `Health` picks the project's own component first: a project that already has a `Health` keeps getting it, and `node.Get<gameplay.Health>()` reaches this one by its full name.

---
id: gameplay/editor
icon: tool.move
parent: gameplay
order: 13
title: Editor tools
---
# Editor tools

The editor builds ready gameplay nodes, draws hit areas and shows the state of controllers, health and state machines during Play.

## Ready nodes

**Create → Gameplay** builds ready nodes:

- Platformer Character: kinematic body, collider, Motion 2D, Health, Hurtbox 2D and Respawn;
- Top-down Character;
- Enemy: dynamic body, Health on team 1 and a periodic Hitbox 2D with knockback;
- Moving Platform, Checkpoint, Kill Zone and Pickup.

## Required components

**Add Component** adds what a component requires. Motion 2D on an empty node brings a Rigid Body 2D, and one Undo removes both. The list shows what comes along. `aego scene add-comp`, Paste and presets add it the same way. Adding Motion 2D to a node with a body makes the body kinematic.

![Add Component on an empty node: the entry lists the Virtual Camera 2D that comes with Camera Follow 2D](shots/add-required.png)

## Areas

- Select a node with a hitbox, hurtbox, kill zone, checkpoint or pickup to drag the corners and edges of its area. Snapping applies, and every drag is one Undo step. A hitbox also shows its knockback as an arrow.
- **Gameplay → Gameplay Areas** shows every area dimmed in the Scene View. During Play the Game View draws active hurtboxes and hitboxes, the zones and a health bar over every node with Health.

## Inspector during Play

The runtime state of the controller shows its velocity, whether it is grounded, wall and ceiling contact, the floor and the body under it. Motion 2D adds a contact summary, Health a bar with the hit points and State Machine the current state and how long it has lasted. Lifetime shows the seconds left.

## Profiling and problems

- **Profiler → Gameplay** shows controllers and casts per fixed step, and the hits, deaths, respawns and pickups so far.
- The Event Monitor lists state changes, damage (with the amount up to 9), deaths, respawns and pickups as `gameplay` rows.
- Problems lists a Motion 2D whose body is not kinematic ([GMP001](ref:GMP001), blocks Play), a platformer with Gravity Scale 0 and no Jump Speed ([GMP002](ref:GMP002)), a state machine with empty, duplicate or unknown states ([GMP005](ref:GMP005)) and a Hurtbox 2D with no Health on its node or above it ([GMP006](ref:GMP006); hits on it are ignored).

---
id: gameplay/go
icon: code.brackets
parent: gameplay
order: 14
title: Go
---
# Go

    if hp, ok := scene.Get[gameplay.Health](s, node); ok {
      hp.Damage(3, attacker)
    }
    if m, ok := scene.Get[gameplay.Motion2D](s, player); ok {
      m.AddVelocity(mathx.Vec2{X: 6, Y: -4})
    }
    st := gameplay.Of(world).Stats()

The component types and their methods are the same as in scripts. `gameplay.Of(world)` returns the service of a play world; its `Stats` method counts hits, deaths, respawns and pickups since Play started.

State machines call script hooks through the `script.call` contract (`contracts/scriptcall`): `Method(node, name, params)` finds a plain method of a script on a node, `Call` runs it, and a new `Generation` after a hot reload makes old references stale. The save section `gameplay.respawn` holds the respawn points marked Persist.

Motion 2D reads and moves bodies only through the `physics2d.queries` contract: `CastBody` from a moved pose, `Penetration`, `BodyPose`, `BodyVelocity`, `MoveKinematic`, `Teleport` and `Gravity`. A physics module that provides the contract runs the controller unchanged. Root motion reaches the controller through the `motion2d.receiver` contract (`contracts/motion2d`): `AddDisplacement(node, offset)` returns false for a node without Motion 2D.
