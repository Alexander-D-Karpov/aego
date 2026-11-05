---
id: platformer
icon: doc.tutorial
group: start
title: Platformer from scratch
order: 2
---
# Platformer from scratch

This guide builds a small platformer in an empty 2D project: a character that runs and jumps, a camera that follows it, a pit with a checkpoint, coins and an enemy. Every step ends with the scene file as it looks after the step, and every scene and script on this page is run by the documentation checks.

Start with [Your first project](ref:quickstart) if you have not opened the editor yet.

The finished example is a separate, more detailed project at `examples/platformer`. Open it with `aego edit examples/platformer` and load its main scene to inspect the game used by the website demo. Move with A/D or the arrow keys, jump with Space, attack with J and open settings with Esc. F6 toggles lighting and F7 cycles its debug views.

In the Hierarchy, inspect the player's Dust and Sparks emitters and the StartLantern's Smoke and Embers. The landing and attack events trigger bursts. The camera's Bloom adds a screen-space halo, while the lantern's Light2D illuminates nearby Lit surfaces and Light Occluder 2D contours cast shadows. See [particle effects](ref:particles), [materials and lighting](ref:render2d/materials) and [post-processing](ref:render2d.PostProcess) for editable examples and Try It scenes. This guide below starts from simpler shapes rather than reproducing every asset in the finished project.

## 1. Floor and player

Create a project from the **2D** template. In the Hierarchy, create an empty node and name it `Floor`. Give it these components in the Inspector:

- **Sprite Renderer**, Size `30, 1`, a grey Tint;
- **Rigid Body 2D**, Type `static`;
- **Box Collider 2D**, Size `30, 1`.

Move it to `0, 3`. Create a second node, `Player`, with:

- **Sprite Renderer**, Size `0.6, 1`, an orange Tint;
- **Rigid Body 2D**, Type `kinematic`, Fixed Rotation on;
- **Box Collider 2D**, Size `0.6, 1`, Friction `0`;
- **Motion 2D**, Mode `platformer`.

Motion 2D moves a kinematic body: it slides along walls, stands on floors and climbs small steps. Press Play: the player falls and lands on the floor.

```aescene check
scene version=1 {
  node "f1000000000000000000000000000001" name="Floor" enabled=true layers=1 {
    transform {
      position=[0, 3]
    }
    component "engine.SpriteRenderer" version=1 {
      tint=#4b5563ff
      size=[30, 1]
    }
    component "physics.Body2D" version=1 {
      type=static
    }
    component "physics.BoxCollider2D" version=1 {
      size=[30, 1]
    }
  }
  node "f1000000000000000000000000000002" name="Player" enabled=true layers=1 {
    component "engine.SpriteRenderer" version=1 {
      tint=#f59e0bff
      size=[0.6, 1]
    }
    component "physics.Body2D" version=1 {
      type=kinematic
      fixedRotation=true
    }
    component "physics.BoxCollider2D" version=1 {
      friction=0
      size=[0.6, 1]
    }
    component "gameplay.Motion2D" version=1 {
      mode=platformer
    }
  }
  node "f1000000000000000000000000000003" name="Camera" enabled=true layers=1 {
    component "engine.Camera2D" version=1 {
      zoom=36
    }
  }
}
```

## 2. Run and jump

Create a script `Player` in `scripts/game` and attach it to the player:

```aes check
package game

import "aego/input"

script Player {
	func Update(dt float) {
		m := node.Get<Motion2D>()
		m.Move(input.Axis("Move"))
		if input.Pressed("Jump") {
			m.Jump()
		}
		if input.Released("Jump") {
			m.JumpReleased()
		}
	}
}
```

`Move` and `Jump` are input actions of the 2D template: the arrows or A and D, and Space, or a gamepad. `Move` takes a value from -1 to 1 and Motion 2D accelerates to that share of its Max Speed. `JumpReleased` cuts the jump short when the button is let go early, so a tap makes a small hop. Jump Height, Max Speed and Coyote Time are fields of [Motion 2D](ref:gameplay.Motion2D).

```aescene check
scene version=1 {
  node "f1000000000000000000000000000001" name="Floor" enabled=true layers=1 {
    transform {
      position=[0, 3]
    }
    component "engine.SpriteRenderer" version=1 {
      tint=#4b5563ff
      size=[30, 1]
    }
    component "physics.Body2D" version=1 {
      type=static
    }
    component "physics.BoxCollider2D" version=1 {
      size=[30, 1]
    }
  }
  node "f1000000000000000000000000000002" name="Player" enabled=true layers=1 {
    component "engine.SpriteRenderer" version=1 {
      tint=#f59e0bff
      size=[0.6, 1]
    }
    component "physics.Body2D" version=1 {
      type=kinematic
      fixedRotation=true
    }
    component "physics.BoxCollider2D" version=1 {
      friction=0
      size=[0.6, 1]
    }
    component "gameplay.Motion2D" version=1 {
      mode=platformer
    }
    component "engine.Script" version=1 {
      script "game.game.Player" enabled=true {
      }
    }
  }
  node "f1000000000000000000000000000003" name="Camera" enabled=true layers=1 {
    component "engine.Camera2D" version=1 {
      zoom=36
    }
  }
}
```

The scene names a script by module, package and name. Here the module is `game`; in your project it is the module in `project.aego`.

## 3. A camera that follows

Select the camera and add **Virtual Camera 2D** and **Camera Follow 2D**. Drag the player from the Hierarchy onto the Target field of Camera Follow 2D. The camera now keeps the player in view and catches up smoothly; Deadzone and Damping set how loosely. See [Cameras](ref:camera) for zones, shake and blends.

```aescene check
scene version=1 {
  node "f1000000000000000000000000000001" name="Floor" enabled=true layers=1 {
    transform {
      position=[0, 3]
    }
    component "engine.SpriteRenderer" version=1 {
      tint=#4b5563ff
      size=[30, 1]
    }
    component "physics.Body2D" version=1 {
      type=static
    }
    component "physics.BoxCollider2D" version=1 {
      size=[30, 1]
    }
  }
  node "f1000000000000000000000000000002" name="Player" enabled=true layers=1 {
    component "engine.SpriteRenderer" version=1 {
      tint=#f59e0bff
      size=[0.6, 1]
    }
    component "physics.Body2D" version=1 {
      type=kinematic
      fixedRotation=true
    }
    component "physics.BoxCollider2D" version=1 {
      friction=0
      size=[0.6, 1]
    }
    component "gameplay.Motion2D" version=1 {
      mode=platformer
    }
    component "engine.Script" version=1 {
      script "game.game.Player" enabled=true {
      }
    }
  }
  node "f1000000000000000000000000000003" name="Camera" enabled=true layers=1 {
    component "engine.Camera2D" version=1 {
      zoom=36
    }
    component "camera.VirtualCamera2D" version=1 {
    }
    component "camera.CameraFollow2D" version=1 {
      target=node("f1000000000000000000000000000002")
    }
  }
}
```

## 4. A pit and a checkpoint

Split the floor in two with a gap between them. Under the gap, add a node `Pit` with a **Kill Zone 2D** whose Area covers the bottom of the level. Give the player **Health** and **Respawn**. Add a node `Checkpoint` past the pit with a **Checkpoint 2D**.

Falling into the pit kills the player. After Respawn's Delay it comes back at the last checkpoint it touched, or where it started, with full health.

```aescene check
scene version=1 {
  node "f1000000000000000000000000000001" name="Floor" enabled=true layers=1 {
    transform {
      position=[-6, 3]
    }
    component "engine.SpriteRenderer" version=1 {
      tint=#4b5563ff
      size=[16, 1]
    }
    component "physics.Body2D" version=1 {
      type=static
    }
    component "physics.BoxCollider2D" version=1 {
      size=[16, 1]
    }
  }
  node "f1000000000000000000000000000004" name="Floor 2" enabled=true layers=1 {
    transform {
      position=[13, 3]
    }
    component "engine.SpriteRenderer" version=1 {
      tint=#4b5563ff
      size=[16, 1]
    }
    component "physics.Body2D" version=1 {
      type=static
    }
    component "physics.BoxCollider2D" version=1 {
      size=[16, 1]
    }
  }
  node "f1000000000000000000000000000005" name="Pit" enabled=true layers=1 {
    transform {
      position=[3.5, 8]
    }
    component "gameplay.KillZone2D" version=1 {
      area=[-30, -1, 60, 2]
    }
  }
  node "f1000000000000000000000000000006" name="Checkpoint" enabled=true layers=1 {
    transform {
      position=[8, 1.5]
    }
    component "engine.SpriteRenderer" version=1 {
      tint=#22c55eff
      size=[0.15, 2]
    }
    component "gameplay.Checkpoint2D" version=1 {
    }
  }
  node "f1000000000000000000000000000002" name="Player" enabled=true layers=1 {
    component "engine.SpriteRenderer" version=1 {
      tint=#f59e0bff
      size=[0.6, 1]
    }
    component "physics.Body2D" version=1 {
      type=kinematic
      fixedRotation=true
    }
    component "physics.BoxCollider2D" version=1 {
      friction=0
      size=[0.6, 1]
    }
    component "gameplay.Motion2D" version=1 {
      mode=platformer
    }
    component "gameplay.Health" version=1 {
      max=3
    }
    component "gameplay.Respawn" version=1 {
    }
    component "engine.Script" version=1 {
      script "game.game.Player" enabled=true {
      }
    }
  }
  node "f1000000000000000000000000000003" name="Camera" enabled=true layers=1 {
    component "engine.Camera2D" version=1 {
      zoom=36
    }
    component "camera.VirtualCamera2D" version=1 {
    }
    component "camera.CameraFollow2D" version=1 {
      target=node("f1000000000000000000000000000002")
    }
  }
}
```

## 5. Coins

A coin is a node with a **Pickup 2D**. It is collected when a node with Motion 2D enters its Area, and then removed. To count coins, give the player a counter:

```aes check
package game

import (
	"aego/input"
	"aego/log"
)

script Player {
	var coins int

	func Update(dt float) {
		m := node.Get<Motion2D>()
		m.Move(input.Axis("Move"))
		if input.Pressed("Jump") {
			m.Jump()
		}
		if input.Released("Jump") {
			m.JumpReleased()
		}
	}

	func AddCoin() {
		coins++
		log.Info("coins: " + string(coins))
	}
}
```

and give each coin a script that tells the player:

```aes check
package game

script Coin {
	func Ready() {
		node.Get<Pickup2D>().Collected.Connect(OnCollected)
	}

	func OnCollected(by NodeRef) {
		p := by.GetScript<Player>()
		if p != nil {
			p.AddCoin()
		}
	}
}
```

Make one coin, then turn it into a prefab (**Create Prefab** in the Hierarchy menu) and place copies of it; a change to the prefab reaches every copy. See [Prefabs](ref:prefabs).

```aescene check
scene version=1 {
  node "f1000000000000000000000000000001" name="Floor" enabled=true layers=1 {
    transform {
      position=[-6, 3]
    }
    component "engine.SpriteRenderer" version=1 {
      tint=#4b5563ff
      size=[16, 1]
    }
    component "physics.Body2D" version=1 {
      type=static
    }
    component "physics.BoxCollider2D" version=1 {
      size=[16, 1]
    }
  }
  node "f1000000000000000000000000000004" name="Floor 2" enabled=true layers=1 {
    transform {
      position=[13, 3]
    }
    component "engine.SpriteRenderer" version=1 {
      tint=#4b5563ff
      size=[16, 1]
    }
    component "physics.Body2D" version=1 {
      type=static
    }
    component "physics.BoxCollider2D" version=1 {
      size=[16, 1]
    }
  }
  node "f1000000000000000000000000000005" name="Pit" enabled=true layers=1 {
    transform {
      position=[3.5, 8]
    }
    component "gameplay.KillZone2D" version=1 {
      area=[-30, -1, 60, 2]
    }
  }
  node "f1000000000000000000000000000006" name="Checkpoint" enabled=true layers=1 {
    transform {
      position=[8, 1.5]
    }
    component "engine.SpriteRenderer" version=1 {
      tint=#22c55eff
      size=[0.15, 2]
    }
    component "gameplay.Checkpoint2D" version=1 {
    }
  }
  node "f1000000000000000000000000000007" name="Coin" enabled=true layers=1 {
    transform {
      position=[2, 2]
    }
    component "engine.SpriteRenderer" version=1 {
      tint=#facc15ff
      size=[0.4, 0.4]
    }
    component "gameplay.Pickup2D" version=1 {
    }
    component "engine.Script" version=1 {
      script "game.game.Coin" enabled=true {
      }
    }
  }
  node "f1000000000000000000000000000002" name="Player" enabled=true layers=1 {
    component "engine.SpriteRenderer" version=1 {
      tint=#f59e0bff
      size=[0.6, 1]
    }
    component "physics.Body2D" version=1 {
      type=kinematic
      fixedRotation=true
    }
    component "physics.BoxCollider2D" version=1 {
      friction=0
      size=[0.6, 1]
    }
    component "gameplay.Motion2D" version=1 {
      mode=platformer
    }
    component "gameplay.Health" version=1 {
      max=3
    }
    component "gameplay.Respawn" version=1 {
    }
    component "engine.Script" version=1 {
      script "game.game.Player" enabled=true {
      }
    }
  }
  node "f1000000000000000000000000000003" name="Camera" enabled=true layers=1 {
    component "engine.Camera2D" version=1 {
      zoom=36
    }
    component "camera.VirtualCamera2D" version=1 {
    }
    component "camera.CameraFollow2D" version=1 {
      target=node("f1000000000000000000000000000002")
    }
  }
}
```

## 6. An enemy

An enemy walks back and forth and hurts the player on contact. Give it the same body as the player, a **Motion 2D** and a **Hitbox 2D** with Team `1`, so it never hurts other enemies on team 1. Give the player a **Hurtbox 2D**: the hitbox damages the Health of the node whose hurtbox it touches. The player then blinks for Invuln Time and cannot be hit again until it ends.

The enemy turns around at walls:

```aes check
package game

script Patrol {
	@export
	var Speed float = 0.5

	var dir float = 1

	func Ready() {
		node.Get<Motion2D>().HitWall.Connect(OnWall)
	}

	func OnWall(normal Vec2) {
		if normal.X != 0 {
			dir = normal.X
		}
	}

	func FixedUpdate(dt float) {
		node.Get<Motion2D>().Move(dir * Speed)
	}
}
```

`HitWall` gives the wall's normal, which points away from the wall, so its X is the new direction.

```aescene check
scene version=1 {
  node "f1000000000000000000000000000001" name="Floor" enabled=true layers=1 {
    transform {
      position=[-6, 3]
    }
    component "engine.SpriteRenderer" version=1 {
      tint=#4b5563ff
      size=[16, 1]
    }
    component "physics.Body2D" version=1 {
      type=static
    }
    component "physics.BoxCollider2D" version=1 {
      size=[16, 1]
    }
  }
  node "f1000000000000000000000000000004" name="Floor 2" enabled=true layers=1 {
    transform {
      position=[13, 3]
    }
    component "engine.SpriteRenderer" version=1 {
      tint=#4b5563ff
      size=[16, 1]
    }
    component "physics.Body2D" version=1 {
      type=static
    }
    component "physics.BoxCollider2D" version=1 {
      size=[16, 1]
    }
  }
  node "f1000000000000000000000000000009" name="Wall" enabled=true layers=1 {
    transform {
      position=[20.5, 1.5]
    }
    component "engine.SpriteRenderer" version=1 {
      tint=#4b5563ff
      size=[1, 2]
    }
    component "physics.Body2D" version=1 {
      type=static
    }
    component "physics.BoxCollider2D" version=1 {
      size=[1, 2]
    }
  }
  node "f1000000000000000000000000000005" name="Pit" enabled=true layers=1 {
    transform {
      position=[3.5, 8]
    }
    component "gameplay.KillZone2D" version=1 {
      area=[-30, -1, 60, 2]
    }
  }
  node "f1000000000000000000000000000006" name="Checkpoint" enabled=true layers=1 {
    transform {
      position=[8, 1.5]
    }
    component "engine.SpriteRenderer" version=1 {
      tint=#22c55eff
      size=[0.15, 2]
    }
    component "gameplay.Checkpoint2D" version=1 {
    }
  }
  node "f1000000000000000000000000000007" name="Coin" enabled=true layers=1 {
    transform {
      position=[2, 2]
    }
    component "engine.SpriteRenderer" version=1 {
      tint=#facc15ff
      size=[0.4, 0.4]
    }
    component "gameplay.Pickup2D" version=1 {
    }
    component "engine.Script" version=1 {
      script "game.game.Coin" enabled=true {
      }
    }
  }
  node "f1000000000000000000000000000008" name="Enemy" enabled=true layers=1 {
    transform {
      position=[14, 2]
    }
    component "engine.SpriteRenderer" version=1 {
      tint=#ef4444ff
      size=[0.8, 0.8]
    }
    component "physics.Body2D" version=1 {
      type=kinematic
      fixedRotation=true
    }
    component "physics.BoxCollider2D" version=1 {
      friction=0
      size=[0.8, 0.8]
    }
    component "gameplay.Motion2D" version=1 {
      mode=platformer
    }
    component "gameplay.Hitbox2D" version=1 {
      area=[-0.4, -0.4, 0.8, 0.8]
      team=1
      active=true
    }
    component "engine.Script" version=1 {
      script "game.game.Patrol" enabled=true {
      }
    }
  }
  node "f1000000000000000000000000000002" name="Player" enabled=true layers=1 {
    component "engine.SpriteRenderer" version=1 {
      tint=#f59e0bff
      size=[0.6, 1]
    }
    component "physics.Body2D" version=1 {
      type=kinematic
      fixedRotation=true
    }
    component "physics.BoxCollider2D" version=1 {
      friction=0
      size=[0.6, 1]
    }
    component "gameplay.Motion2D" version=1 {
      mode=platformer
    }
    component "gameplay.Health" version=1 {
      max=3
    }
    component "gameplay.Hurtbox2D" version=1 {
      area=[-0.3, -0.5, 0.6, 1]
    }
    component "gameplay.Respawn" version=1 {
    }
    component "engine.Script" version=1 {
      script "game.game.Player" enabled=true {
      }
    }
  }
  node "f1000000000000000000000000000003" name="Camera" enabled=true layers=1 {
    component "engine.Camera2D" version=1 {
      zoom=36
    }
    component "camera.VirtualCamera2D" version=1 {
    }
    component "camera.CameraFollow2D" version=1 {
      target=node("f1000000000000000000000000000002")
    }
  }
}
```

## 7. Play it anywhere

**File → Export…** builds the game for Linux, Windows, macOS or the web, as in [Your first project](ref:quickstart). On a phone, the web build shows a touch gamepad: the stick moves and A jumps, because the actions are also bound to a gamepad.

## Where to go next

- [Recipes](ref:recipes): double jump, one-way platforms, moving platforms, dash and more.
- [Gameplay kit](ref:gameplay): every gameplay component in one place.
- [Animation](ref:animation): sprite animation and state machines for the player.
- `examples/platformer` in the engine source is this game grown further: animation, sound, stomping, saves, settings and 2D lighting.

## Lighting and surface assets in the example

Open the existing `examples/platformer` project to inspect the complete lighting example. Its level, collisions and controls remain the gameplay example; the added surface assets and lights make normal mapping, shadows and emission visible in that same level. F6 toggles lighting for the view. F7 cycles the lighting, normal and shadow views. Both keys are registered Input Actions and do not change materials or simulation state.

| Existing asset | Surface association |
|---|---|
| `assets/surfaces/platform.png` | Its explicit Height source is `platform_height.png`, with import-time normal generation enabled. This generated normal is shared by the level's platforms. |
| `ground.png` and `wall.png` | Explicit normal maps take priority; their height sources remain linked for authoring. |
| `runner.png` | Each of the eight original animation frames has its own stable normal-frame association in `runner_n.png`. The source silhouette, frame size and gameplay origin are preserved. |
| `lamp.png` | `lamp_em.png` is an emission mask. It brightens the lamp's core using `lamp_lit.aemat`; the nearby Point light illuminates other objects. The mask does not create Bloom. |
| `window_cookie.png` | A linear cookie mask shapes the Sprite light without using the albedo atlas coordinates. |

To edit the existing platform's surface, open `platform.png` from Assets and follow **Height source** to `platform_height.png`. Enable **Lit surface** and change the height strength in that shared height document. The generated-normal candidate and movable preview light show the result before Save. Press Play, keep the image tab open, and use **Save settings**: the running level receives the new prepared normal generation. Undo restores the earlier appearance while leaving the metadata document dirty and the saved file unchanged; Redo restores the saved result. Stop returns to the unchanged EditScene. An explicit Normal assignment would override this Height-generated normal.

Open `runner.png`, select an existing frame and use the preview's Flip X/Y controls to check the paired normal. These controls affect only the preview; the actual player script sets FlipX when changing direction. The source image and frame pixels are edited in an external image tool, while channel links and import settings use the shared asset document and its Undo history.

The player and enemy use separate `LightOccluder2D` polygons for their shadows. These approximate the head and legs of the shared runner sprite without changing the player's capsule or the enemy's box collider. The outlines are static: they do not follow individual animation frames.

See [Image and Sprite authoring](ref:render2d/image-viewer) and [Materials and shaders](ref:render2d/materials) for role/color-space rules, missing-reference diagnostics and the lighting model. Native and web exports use this same project and include its normal, height-derived, emission and cookie artifacts. The site's demo is prepared from this export; preparation alone does not publish a new site version.

## Dust and sparks in the example

Open `examples/platformer`, jump and keep moving after landing. The dust starts at the player's feet and stays over the patch of floor where it was emitted. Select `Player/Dust` to change the effect: `rectSize` sets the width and depth of the emitting strip, and `lifetime` sets how long each puff lasts. In `scripts/player/player.aes`, `OnLanded` emits eight particles, `OnJumped` five and `OnClimbed` six. `playOnStart` is off and `rate` is zero, so the emitter produces particles only on these events. `Player/Slash/Sparks` emits a burst on the attack animation's Hit event.

`space=world` fixes each particle's birth transform. Use `space=local` for an effect that should travel with its parent. To compare the same level with and without particles, change **Particle Effects** in the game's accessibility settings. Leave F6 and F7 unchanged: they control lighting.

At `StartLantern`, select the `Smoke` or `Embers` child to inspect the two continuous emitters. Use the Inspector preview to pause the effect, lock its seed and compare changes before Play. Under **Over Life**, drag a gradient key, release it and use Undo to restore the previous color transition. See [CPU particle effects](ref:particles) for the five presets and the runnable example.

## Camera PostFX in the example game

Open the platformer in Game View and select **Camera → Post Process**. Bloom is enabled with Threshold 0.75, Intensity 0.22 and Radius 6. Watch the halo around the lantern while changing Intensity. Vignette, Color Grading and the Cold custom material are already in the stack and start disabled. Enable one to compare its effect. Open Settings during Play and compare the opaque menu with the component on and off. To restore the original world image, disable Post Process. F6 and F7 control lighting and its diagnostics.

Center is relative to the camera output; (0.5, 0.5) is its centre. Intensity controls darkening, while Softness controls the width of the transition. The boundary is circular in physical viewport pixels even on a wide display.
