---
id: animation
icon: play.start
group: gameplay
title: Animation
order: 4
---
# Animation

An animation clip changes properties of a node and its children over time: sprite frames, colors, transforms, script fields. An [Animation Player](ref:animation.AnimationPlayer) plays one clip and reports its events to scripts.

Clip time, gameplay events and every track except the transform pose move on fixed steps, like physics. A replay of the same input gives the same frames, events and state hash at any frame rate.

    func OnAnimEvent(id int, payload float) {
      if anim.EventName(id) == "Hit" {
        node.Child("Sword").Get<Hitbox2D>().Pulse()
      }
    }

---
id: animation/clips
icon: file.data
parent: animation
order: 1
title: Clips
---
# Clips

A clip is an `.aeanim` file in the assets folder. It has a length in seconds, a loop mode (`once`, `loop`, `pingPong`) and tracks:

    anim version=1 length=0.6 loop=loop {
      frames "" fps=10 {
        sprites=[asset("…run_0"), asset("…run_1"), asset("…run_2")]
      }
      track "" prop="SpriteRenderer.tint" kind=color {
        key t=0 v=[1, 1, 1, 1] interp=linear {}
        key t=0.3 v=[1, 0.6, 0.6, 1] {}
      }
      track "Arm" prop="transform.rotation" kind=float32 authority=simulation {
        key t=0 v=0 {}
        key t=0.3 v=0.8 tangent=linear {}
      }
      event t=0.3 name="Hit" payload=1 {}
    }

| Part | Meaning |
|---|---|
| label of `track` | the node, as a path from the player's node; `""` is the node itself |
| `prop` | the property, with the paths of tweens: `Component.Field`, `Component.Field.a`, `transform.position`, `script/Name.Field` |
| `kind` | the value type: `float32`, `int32`, `bool`, `enum`, `vec2`, `vec3`, `vec4`, `color`, `rect`, `quat`, `assetRef` |
| `authority` | `presentation` (default) or `simulation`, see [Time and authority](ref:animation/time) |
| `onEnd` | `hold` keeps the last value when the clip ends or stops, `restore` puts back the value from before Play |
| `root=true` | only on `""` with `prop="transform.position"` and `kind=vec2`: the track is not written; an Animator with Root Motion moves the node by its change instead |
| `key` | `t`, `v`, and `interp=bezier` (default), `linear` or `step`; `tangent=auto`, `linear` or `free` with `in` and `out` |
| `frames` | sprite frames: `sprites` and `fps` or `durations`, `flipX` optional; it becomes step tracks on `SpriteRenderer.sprite` and `SpriteRenderer.flipX` |
| `event` | an event at `t` with a declared `name`, an optional `payload` and `mode` |

Numbers, vectors and colors interpolate; rotations as quaternions take the short arc; `int` steps unless the track has `numeric=true`; `bool`, `enum` and asset references step. Sprites are sprite assets, usually members of an atlas group.

The engine ships four clips for tests and sketches: `engine/anim/run`, eight frames of a running figure (`engine/sprites/runner_0` to `_7`) with two `Footstep` presentation events; `engine/anim/bob`, a simulation sway of `transform.rotation` with a `Hit` gameplay event at the right turn; `engine/anim/jump`, the runner's stride pose; and `engine/anim/hit`, one 0.3 s swing of `transform.rotation`. The built-in controller plays jump in its Jump state and hit in its Hit state.

A clip that does not import keeps its last good version, and the error is in Problems: [ANM001](ref:ANM001) for a track or file the importer cannot read, [ANM002](ref:ANM002) for a key or event outside the length, [ANM003](ref:ANM003) for an event that is not declared.

---
id: animation/player
try: AnimationPlayerOriginal.aescene
try_select: animation.AnimationPlayer
icon: play.start
parent: animation
order: 2
title: Animation Player
---
# Animation Player

[Animation Player](ref:animation.AnimationPlayer) plays its Clip, from the start of Play when Play On Start is set. Speed scales time; a negative speed plays backwards. Loop overrides the loop mode of the clip.

In Try it, the runner plays the built-in run clip and the pendulum plays a separate bob clip. Press R to replay the runner or B to reverse it; their scripts show presentation and gameplay events.

| Method | |
|---|---|
| `Play(clip)` | start a clip from the beginning; `nil` plays the current clip again |
| `Stop()` | end the clip; `restore` tracks get their old values, `Finished(-1)` runs |
| `Pause()`, `Resume()` | hold and continue |
| `Seek(t)` | jump to a time in the clip without firing the events in between |
| `SetSpeed(s)` | change the speed |

| Signal | |
|---|---|
| `Finished(cycle int)` | a clip that does not loop ended (`0`), or `Stop` (`-1`) |
| `Event(id int, payload float)` | the clip reached an event |

Commands take effect on the next fixed step, so a script may call them from Update. Time, Playing and Cycle show the state during Play.

A track whose node is missing ([ANM005](ref:ANM005)) or whose property does not match its kind ([ANM001](ref:ANM001)) is off; the rest of the clip plays. When a clip is imported again with the same tracks, the players keep their place; with other tracks they stop ([ANM008](ref:ANM008)).

---
id: animation/time
icon: panel.profiler
parent: animation
order: 3
title: Time and authority
---
# Time and authority

A player in game time advances on fixed steps and stops while the game is paused. On every step it writes its tracks at the clip time of that step. One exception: tracks on the transform of a 2D node with `authority=presentation` (the default) are shown at the time between two steps, so motion stays smooth at any frame rate, without changing the node's position in the game.

| Authority | On a transform | On other properties |
|---|---|---|
| `presentation` | drawn only; scripts, physics and saves see the transform without the animation | written on fixed steps |
| `simulation` | the node moves for real; a kinematic body is moved with its velocity, so it carries what stands on it | written on fixed steps |

Animation and other systems never drive one property together:

- A tween on a property the animation drives stops with `Completed(false)`.
- A track on the transform of a node with [Motion 2D](ref:gameplay.Motion2D), or a second track on a property another track drives, is off ([ANM004](ref:ANM004)).
- On a dynamic body every transform track is off; on a kinematic body a presentation transform track is off; a static body warns that it is teleported ([ANM006](ref:ANM006)).

With Time Domain `unscaled` the player runs in real time and keeps playing while the game is paused, for menus and other presentation. It is not part of the game state: simulation tracks are off and gameplay events arrive as presentation events ([ANM007](ref:ANM007)).

---
id: animation/events
icon: status.busy
parent: animation
order: 4
title: Events
---
# Events

Events are declared in Project Settings → Animation, or in `project.aego`:

    animation {
      event "Hit" payload=float {}
      event "Footstep" {}
    }

`Footstep` and `Hit` (payload `float`) are declared by the engine for its built-in clips; a project may use them without declaring them.

An event in a clip has a mode:

| Mode | When it runs |
|---|---|
| `gameplay` (default) | on the fixed step that crosses it, once per cycle, the same on every replay |
| `presentation` | when the drawn time crosses it; for sounds and effects; not during a replay |
| `preview` | only in the editor's preview |

The scripts of the player's node get `OnAnimEvent(id int, payload float)`, and the player emits its `Event(id, payload)` signal. An event at time 0 runs when the clip starts and on every new cycle; an event at the end runs once per cycle. A step that covers several cycles runs each event once per cycle, in order. `Seek` fires nothing.

---
id: section/animation
icon: animation.player
---
## Animation events

Declare project-specific animation event names in the `animation` block. `Footstep` and `Hit` are built in; redeclaring either does not replace the built-in event. Each project event needs a unique, nonempty name. `payload` is optional and can be `none`, `int`, `float` or `bool`; its default is `none`.

```aetext
animation {
  event "Land" payload=float {}
  event "OpenDoor" {}
}
```

In **Project Settings → Animation**, add, rename or remove project events and choose their payload. Changes stay in the settings draft until **Apply**; Undo and Redo work on that draft. The new declarations take effect on the next Play. Renaming or removing a declaration does not rewrite animation clips: update their event names too.

Animation clips refer to these names. Scripts receive `OnAnimEvent(id, payload)` and can resolve names with `anim.EventName` and `anim.EventID`; see [Animation events](ref:animation/events). The same block can also be edited in `project.aego`.

---
id: animation/animator
try: AnimatorOriginal.aescene
try_select: animation.Animator
icon: play.start
parent: animation
order: 6
title: Animator
---
# Animator

An [Animator](ref:animation.Animator) plays the states of a controller, an `.aectrl` file: each state has a clip, and transitions switch between states when parameters change. Scripts set the parameters; the graph does the rest.

In Try it, use A/D to walk, Space to jump and J to trigger Hit. The script sets controller parameters and shows the current state.

    func FixedUpdate(dt float) {
      m := node.Get<Motion2D>()
      a := node.Get<Animator>()
      a.SetFloat("Speed", math.Abs(m.Velocity.X))
      a.SetBool("Grounded", m.Grounded)
      if input.Pressed("Attack") {
        a.SetTrigger("Attack")
      }
    }

| Method | |
|---|---|
| `SetBool`, `SetInt`, `SetFloat(name, value)` | set a parameter; transitions read it on the next fixed step |
| `SetTrigger(name)`, `ResetTrigger(name)` | a one-shot parameter: the transition that uses it, or the end of that step, clears it |
| `GetBool`, `GetInt`, `GetFloat(name)` | read a parameter |
| `Play(state)` | switch to a state on the next fixed step, without a blend |
| `CrossFade(state, seconds)` | blend into a state |
| `IsState(name) bool`, `StateName(index) string` | the state, or the name of a state index |

| Signal | |
|---|---|
| `StateEntered(state int)` | a transition into the state started |
| `StateExited(state int)` | the state stopped playing, at the end of the blend |
| `Event(id int, payload float)` | a state reached an event of its clip |

State, Time, Progress and Blending show the state during Play. Create → Gameplay → Platformer Character comes with an Animator on the built-in controller `engine/anim/platformer`: Idle, Run, Jump and Hit, with the parameters Speed, Grounded and Attack.

Set parameters in `FixedUpdate` when they come from the game state, as above: the graph runs on fixed steps, so it then sees the same values on every replay at any frame rate.

## Animator and Motion 2D together

The platformer example drives its player from one `FixedUpdate`: the controller moves, the animator shows it. Both read the same step, so a replay at any frame rate gives the same states and the same hits.

    func FixedUpdate(dt float) {
      m := node.Get<Motion2D>()
      m.Move(input.Axis("Move"))
      if input.Pressed("Jump") {
        m.Jump()
      }
      a := node.Get<Animator>()
      a.SetFloat("Speed", math.Abs(m.Velocity.X))
      a.SetBool("Grounded", m.Grounded)
      a.SetFloat("VerticalSpeed", m.Velocity.Y)
      if input.Pressed("Attack") {
        a.SetTrigger("Attack")
      }
    }

The values describe the previous step: scripts run before Motion 2D moves. Damage reaches the animator through a signal, `Health.Damaged` → `SetTrigger("Hurt")`, and an `any` transition with `interrupt=true` shows it even in the middle of a blend. The sprite's Flip X stays with the script: it follows the input, not a clip.

In `examples/topdown` the work is split the other way: a State Machine decides what an enemy does (Patrol, Chase, Dead) and its hooks only set Animator parameters (`Chasing`, `Die`); the clips decide how it looks.

---
id: animation/controller
icon: panel.tasks
parent: animation
order: 7
title: Controllers
---
# Controllers

    controller version=1 {
      entry="Idle"
      param "Speed" kind=float {}
      param "Grounded" kind=bool default=true {}
      param "Attack" kind=trigger {}
      state "Idle" clip=asset("…idle") {}
      state "Run" clip=asset("…run") speed=1.2 {}
      state "Hit" clip=asset("…hit") loop=once events=both {}
      transition "Idle" to="Run" duration=0.1 {
        cond "Speed" op=gt value=0.1 {}
      }
      transition "Run" to="Idle" duration=0.1 {
        cond "Speed" op=le value=0.1 {}
      }
      transition "any" to="Hit" interrupt=true {
        cond "Attack" {}
      }
      transition "Hit" to="Idle" exitTime=1 duration=0.05 {}
    }

| Part | Meaning |
|---|---|
| `param` | `kind` is `bool`, `int`, `float` or `trigger`; `default` for the first three |
| `state` | `clip`, `speed`, `loop` (`once`, `loop`, `pingPong`; default from the clip), `events` (see below) |
| `entry` | the state the animator starts in |
| `transition "From" to="To"` | `"any"` as the source leaves every state but the target |
| `cond` | `op` is `eq`, `ne`, `gt`, `ge`, `lt` or `le` with a `value`; without `op` a bool or a trigger must be set |
| `duration` | seconds of the blend; 0 switches at once |
| `exitTime` | the transition waits until the state has played this many passes of its clip: 1 is one pass |
| `interrupt` | the transition may cut into a blend that is running |

Every fixed step the animator advances its states, then looks for a transition: first the `any` transitions in their order, then those of the current state. All conditions must hold. During a blend only `interrupt` transitions are considered. Transitions with no duration may follow each other, up to eight in one step; one with a duration ends the step. The importer checks names, operators and loops of instant transitions ([ANC001](ref:ANC001), [ANC002](ref:ANC002), [ANC003](ref:ANC003), [ANC005](ref:ANC005)); a state whose clip is missing plays nothing ([ANC004](ref:ANC004)).

## Blends

A blend mixes the outgoing and the incoming state by a weight that follows `ease` of the controller, `inOutQuad` by default. Numbers, vectors and colours interpolate, rotations take the short arc, sprites and other steps switch at half weight. A property that only one of the two clips animates blends from the value it had when the transition started. When an `interrupt` transition cuts into a blend, the pose on screen at that moment becomes the outgoing side, so nothing jumps.

## Events during a blend

| `events` of the incoming state | Events that run |
|---|---|
| `dominant` (default) | those of the state whose weight is at least half at the moment of the event |
| `both` | those of both states |
| `incoming` | those of the incoming state only |

An event at the start of the incoming state happens at weight 0, so with `dominant` it belongs to the outgoing state: put the hit of an attack where the attack is already in full view.

---
id: animation/root
icon: gameplay.motion
parent: animation
order: 8
title: Root motion
---
# Root motion

A clip may move the character itself: its root track is `track "" prop="transform.position" kind=vec2 root=true`. An Animator does not write that track; it moves the node by how much the track changes in each step, so a walk cycle advances the character instead of snapping back at every loop.

| Root Motion | What moves |
|---|---|
| `none` | nothing; the root track is ignored |
| `toMotion` | the node, through its [Motion 2D](ref:gameplay.Motion2D): walls stop it, slopes and steps carry it, the velocity stays as it is |
| `toTransform` | the node's position; a kinematic body is moved by its velocity |

Root Scale multiplies the motion in the parent's space: set its X to -1 while the character faces left, since flipping the sprite does not turn the node. With `toMotion` and no Motion 2D on the node, Root Fallback moves the transform instead; without it the motion is dropped ([ANM009](ref:ANM009)). During a blend the motion of the two states is mixed by the blend weight. Root motion runs in game time only.

---
id: animation/go
icon: code.brackets
parent: animation
order: 9
title: Go
---
# Go

    p, _ := sdk.Get[sdk.AnimationPlayer](scene, node)
    p.Play(sdk.AnimClip{ID: runClip})
    sdk.AnimationOf(ctx).SetParam(hero, "Speed", 3)

`sdk.AnimationOf(ctx)` returns the service of the play world; `SetParam(node, name, value)` sets a parameter of an Animator by name, `true` and `false` as 1 and 0. The systems `animation.fixed` (PrePhysics, after `script.fixed` and the anchor `engine.intents`, before `engine.motion`) and `animation.sample` (Animation, always) advance and draw players and animators. A Go system that sets parameters or commands runs before `engine.intents`; a system that reads root motion runs after `engine.motion`.

Root motion reaches a character through the `motion2d.receiver` contract (`contracts/motion2d`), which Motion 2D provides; another controller can provide it too.

---
id: animation/attack
icon: gameplay.hitbox
parent: animation
order: 10
title: Attack with a hit frame
---
# Attack with a hit frame

The damage of an attack belongs to one frame of its clip, not to the button press: put a gameplay event on that frame and pulse a hitbox when it arrives.

    anim version=1 length=0.3 {
      frames "" {
        sprites=[asset("…swing_0"), asset("…swing_1"), asset("…swing_2")]
        durations=[0.06, 0.06, 0.18]
      }
      event t=0.1 name="Hit" payload=1 {}
    }

The controller enters the attack from any state on a trigger and leaves after one pass:

    transition "any" to="Hit" interrupt=true {
      cond "Attack" {}
    }
    transition "Hit" to="Idle" exitTime=1 duration=0.05 {}

The script presses the trigger and turns the event into damage:

    func Ready() {
      hit = anim.EventID("Hit")
    }

    func OnAnimEvent(id int, payload float) {
      if id == hit {
        Slash.Get<Hitbox2D>().Pulse()
      }
    }

- Give the Hitbox 2D `active=false` and `policy=oncePerActivation`: `Pulse` deals one round, so each attack hits each target once.
- A second press during the attack does nothing, since an `any` transition never leaves its own target.
- The event is a gameplay event: it fires on the fixed step that crosses it, the same on every replay.
- Keep the transition into the attack without `duration`: with the default event policy an event during a blend belongs to the side with more weight, and an event early in a fade-in would go to the outgoing state.

---
id: animation/panel
icon: anim.key
parent: animation
order: 11
title: The Animation panel
---
# The Animation panel

![The attack clip in Preview: the player lunges and the slash arc shows its full frame](shots/attack-preview.png)

Window → Animation edits the clip of the selected node's Animation Player, or the clips of its Animator's controller. Create → Animation → Clip (Ctrl+Shift+N, `clip`, Enter) makes a one-second looping clip next to the scene and gives a node without a player one that plays it; Create → Animation → Controller does the same with an Animator and a controller holding one Idle state. The file is new, so only the added component is an Undo step.

**Preview** shows the clip on the node at the time cursor without touching the scene: saves, Play and Undo see the values the scene really has. Fields the clip drives turn read-only in the Inspector, with a yellow bar and the tip "driven by clip". **Rec** turns edits into keys: a value typed in the Inspector or a move in the Scene View sets a key at the cursor instead of changing the scene. With Auto Key off, press K.

The dope sheet shows one row per track and a diamond per key; sprite tracks show the frames as thumbnails when there is room. Drag keys to move them, drag the ruler to scrub, double-click the event lane to add an event. The wheel zooms the time around the mouse, Shift+wheel pans. **Curves** draws the selected numeric track as a curve, one lane at a time (x/y, r/g/b/a); drag points and tangent handles, double-click to add a key, right-click to remove one. A whole drag is one Undo step, and the other lanes of an edited key keep their values.

| Key | Does |
|---|---|
| Space | plays or pauses the clip |
| K | keys every field of the selected node whose value differs from the clip at the cursor |
| Delete | deletes the selected keys |
| Ctrl+C, Ctrl+V | copy keys, paste them at the cursor; pasting into another clip creates the missing tracks |

The keys only work while the mouse is over the panel. The key menu also has **Set from Scene**, which gives the selected keys the values their properties have in the scene.

Drag a sprite or sheet cells from Assets onto a sprite track to key them: the first lands where you let go, the others follow one per 1/12 s, and the clip grows when they run past its end. In Curves the graph shares the ruler of the dope sheet, so a key sits under its time.

To hide a sprite for part of a clip, key its `tint` to a transparent color: `#00000000` and `#ffffff00` both draw nothing, in every blend mode. A fade from `#ffffffff` to `#ffffff00` keeps the color white while it fades; a fade to `#00000000` passes through grey. The platformer's attack shows its slash arc this way, and the player script hides the arc again when an attack is cut short.

In Preview and Record the Scene View draws the path of the node's `transform.position` track, with its keys as dots: drag a dot to move that key. Preview events flash on the path when the playback crosses them. While you drag the move gizmo in Record, the node follows the mouse instead of the clip, and the key is set when you let go.

---
id: animation/sheets
icon: anim.slice
parent: animation
order: 12
title: Sprite sheets and flipbooks
---
# Sprite sheets and flipbooks

Select a PNG in Assets to open its inspector: the sheet with its cells.

- **Slice by Grid** cuts cells of one size, after an offset and with padding between them, row by row. **Count** stops after that many.
- **Slice by Alpha** finds every separate shape whose alpha is above the threshold and orders them in rows from the top, each row from the left.
- **Slice** writes the cells into the sheet's `.meta`. Each cell becomes a sprite asset named `assets/hero.png#hero_000` with its own id. Undo puts the previous cells back.

Slicing again with the same number of cells keeps every id, so clips and scenes that use the cells stay intact. Deleting the sheet removes its cells. A sliced sheet cannot be packed into an atlas group.

![The inspector of a sheet: Import settings, the cells and the slicing setup](shots/slicer.png)

The **Import** section above the sheet edits how the PNG is imported: filter (nearest for pixel art, linear for smooth art), sRGB, mipmaps, the atlas group with its padding, and whether the edges are extruded. Changes wait until **Apply**, which rewrites the `.meta` and reimports the texture; **Revert** drops them. Apply is one Undo step, the same kind as slicing, so Undo restores both the settings and the cells in the order you changed them.

In Assets a sliced sheet has an arrow on its tile: it opens the cells right after the sheet, each with its thumbnail. Select cells with Ctrl+click and Shift+click, then **Flipbook from Selection** uses them in name order. Drag a cell onto a `sprite` field in the Inspector or onto a sprite track in the Animation panel. Cells cannot be renamed, moved or deleted on their own: slice the sheet again instead.

To animate the frames, select the node, keep the sheet selected in Assets (or open in the inspector) and pick Create → Animation → **Flipbook from Selection** (Ctrl+Shift+N, `flip`, Enter). The clip shows the cells one after another at 12 frames per second, as step keys on `SpriteRenderer.sprite`. A node whose Animation Player already has a clip gets the frames in that clip; otherwise a new looping clip is created next to the scene with an Animation Player that plays it. The Animation panel shows the frames as thumbnails.

---
id: animation/animator-panel
icon: anim.graph
parent: animation
order: 13
title: The Animator panel
---
# The Animator panel

![The Animator panel with the platformer's controller](shots/animator.png)

Window → Animator shows the controller of the selected node's Animator as a graph: one box per state, the **Entry** pill pointing at the state the Animator starts in, and the **Any State** pill for transitions that may leave any state. Arrows are transitions, labelled with their conditions; two transitions between the same states in opposite directions bend apart. The wheel zooms around the mouse; drag the background or hold the middle button to pan; Frame All in the background menu fits the graph.

Drag a state, Entry or Any to move it; the position is saved as `pos` of the state and `entryPos` / `anyPos` of the controller, so the layout belongs to the file. Drag from the dot on the right of a state (or of Any) to another state to add a transition. Right-click the background to add a state or run **Auto Layout**, which places every state in columns from the entry. Right-click a state to make it the entry or delete it with its transitions; right-click a transition to move it up or down or delete it.

The left side edits the selection: a state's clip and speed, or a transition's duration, exit time, interrupt flag and conditions. Transitions are tried in their order, so the arrows ↑ ↓ next to the title move the selected one earlier or later. Under the inspector are the parameters; + adds one of each kind, − removes it.

Every change is one Undo step on the controller file. A change made outside the editor, in a text editor or through `aego mcp`, shows up as soon as the file is reimported.

During Play the state the Animator is in is highlighted, with a ring that fills as its clip plays, and the parameters become live: type a float, toggle a bool or press **Fire** for a trigger to drive the running game.
