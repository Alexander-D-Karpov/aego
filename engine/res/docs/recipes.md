---
id: recipes
icon: doc.recipe
group: start
title: Recipes
order: 3
---
# Recipes

Short answers to common platformer questions. Each recipe is a script and the scene it runs in, built on the player from [Platformer from scratch](ref:platformer); every one is run by the documentation checks.

## Double jump

Motion 2D jumps only from the ground or within Coyote Time. A second jump in the air sets the upward speed directly, and the count resets on landing:
```aes check
package game

import "aego/input"

script Player {
	@export
	var AirJumps int = 1

	@export
	var AirJumpSpeed float = 8

	var left int

	func Ready() {
		node.Get<Motion2D>().Landed.Connect(OnLanded)
	}

	func OnLanded(speed float) {
		left = AirJumps
	}

	func Update(dt float) {
		m := node.Get<Motion2D>()
		m.Move(input.Axis("Move"))
		if input.Released("Jump") {
			m.JumpReleased()
		}
		if input.Pressed("Jump") {
			if m.Grounded {
				m.Jump()
			} else if left > 0 {
				left--
				m.AddVelocity(Vec2{0, -m.Velocity.Y - AirJumpSpeed})
			}
		}
	}
}
```

The velocity is in units per second and up is negative Y, so the added velocity cancels the current vertical speed and sets `AirJumpSpeed` upwards.

```aescene check
scene version=1 {
  node "e1000000000000000000000000000001" name="Floor" enabled=true layers=1 {
    transform {
      position=[0, 3]
    }
    component "engine.SpriteRenderer" version=1 {
      tint=#4b5563ff
      size=[40, 1]
    }
    component "physics.Body2D" version=1 {
      type=static
    }
    component "physics.BoxCollider2D" version=1 {
      size=[40, 1]
    }
  }
  node "e1000000000000000000000000000002" name="Player" enabled=true layers=1 {
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
  node "e1000000000000000000000000000003" name="Camera" enabled=true layers=1 {
    component "engine.Camera2D" version=1 {
      zoom=36
    }
    component "camera.VirtualCamera2D" version=1 {
    }
    component "camera.CameraFollow2D" version=1 {
      target=node("e1000000000000000000000000000002")
    }
  }
}
```

## One-way platforms

A platform the player jumps through from below and stands on from above needs no script. Give it an **Edge Collider 2D** with two points and One Sided on. The player script is the plain one from the guide:

```aes check
package game

import "aego/input"

script Player {
	func Update(dt float) {
		m := node.Get<Motion2D>()
		m.Move(input.Axis("Move"))
		if input.Released("Jump") {
			m.JumpReleased()
		}
		if input.Pressed("Jump") {
			m.Jump()
		}
	}
}
```

The ledge sits over the floor:

```aescene check
scene version=1 {
  node "e1000000000000000000000000000001" name="Floor" enabled=true layers=1 {
    transform {
      position=[0, 3]
    }
    component "engine.SpriteRenderer" version=1 {
      tint=#4b5563ff
      size=[40, 1]
    }
    component "physics.Body2D" version=1 {
      type=static
    }
    component "physics.BoxCollider2D" version=1 {
      size=[40, 1]
    }
  }
  node "e1000000000000000000000000000004" name="Ledge" enabled=true layers=1 {
    transform {
      position=[2, 0.8]
    }
    component "engine.SpriteRenderer" version=1 {
      tint=#94a3b8ff
      size=[3, 0.2]
    }
    component "physics.Body2D" version=1 {
      type=static
    }
    component "physics.EdgeCollider2D" version=1 {
      points=[[-1.5, -0.1], [1.5, -0.1]]
      oneSided=true
    }
  }
  node "e1000000000000000000000000000002" name="Player" enabled=true layers=1 {
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
  node "e1000000000000000000000000000003" name="Camera" enabled=true layers=1 {
    component "engine.Camera2D" version=1 {
      zoom=36
    }
    component "camera.VirtualCamera2D" version=1 {
    }
    component "camera.CameraFollow2D" version=1 {
      target=node("e1000000000000000000000000000002")
    }
  }
}
```

## Moving platforms

A kinematic body moved with `MoveTo` carries a Motion 2D standing on it while Moving Platforms is on, which is the default. This script moves its body back and forth between its start and `Travel`:
```aes check
package game

import "aego/math"

script Lift {
	@export
	var Travel Vec2 = Vec2{4, 0}

	@export
	var Period float = 4

	var t float
	var start Vec2

	func Ready() {
		start = node.Position()
	}

	func FixedUpdate(dt float) {
		t += dt
		k := 0.5 - 0.5*math.Cos(t/Period*2*math.Pi)
		node.Get<Body2D>().MoveTo(Vec2{start.X + Travel.X*k, start.Y + Travel.Y*k}, 0)
	}
}
```

```aescene check
scene version=1 {
  node "e1000000000000000000000000000001" name="Floor" enabled=true layers=1 {
    transform {
      position=[0, 3]
    }
    component "engine.SpriteRenderer" version=1 {
      tint=#4b5563ff
      size=[40, 1]
    }
    component "physics.Body2D" version=1 {
      type=static
    }
    component "physics.BoxCollider2D" version=1 {
      size=[40, 1]
    }
  }
  node "e1000000000000000000000000000005" name="Lift" enabled=true layers=1 {
    transform {
      position=[3, 1.4]
    }
    component "engine.SpriteRenderer" version=1 {
      tint=#0ea5e9ff
      size=[2, 0.3]
    }
    component "physics.Body2D" version=1 {
      type=kinematic
    }
    component "physics.BoxCollider2D" version=1 {
      size=[2, 0.3]
    }
    component "engine.Script" version=1 {
      script "game.game.Lift" enabled=true {
      }
    }
  }
  node "e1000000000000000000000000000002" name="Player" enabled=true layers=1 {
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
  node "e1000000000000000000000000000003" name="Camera" enabled=true layers=1 {
    component "engine.Camera2D" version=1 {
      zoom=36
    }
    component "camera.VirtualCamera2D" version=1 {
    }
    component "camera.CameraFollow2D" version=1 {
      target=node("e1000000000000000000000000000002")
    }
  }
}
```

Platform On Leave on the player's Motion 2D decides what happens to the platform's speed when the player jumps off.

## Dash

`Dash` moves at a fixed velocity for a time and ignores input and gravity meanwhile, unless Dash Gravity is on. A cooldown keeps it from being spammed:
```aes check
package game

import "aego/input"

script Player {
	@export
	var DashSpeed float = 16

	@export
	var DashTime float = 0.15

	@export
	var Cooldown float = 0.6

	var facing float = 1
	var wait float

	func Update(dt float) {
		m := node.Get<Motion2D>()
		m.Move(input.Axis("Move"))
		if input.Released("Jump") {
			m.JumpReleased()
		}
		if input.Pressed("Jump") {
			m.Jump()
		}
		x := input.Axis("Move")
		if x != 0 {
			facing = x
		}
		wait -= dt
		if input.KeyPressed(input.KeyLShift) && wait <= 0 {
			m.Dash(Vec2{facing * DashSpeed, 0}, DashTime)
			wait = Cooldown
		}
	}
}
```

The key is read directly here. In a game, add a `Dash` action in **Project Settings → Input** and read it with `input.Pressed("Dash")`, so it can be rebound and works on a gamepad.

```aescene check
scene version=1 {
  node "e1000000000000000000000000000001" name="Floor" enabled=true layers=1 {
    transform {
      position=[0, 3]
    }
    component "engine.SpriteRenderer" version=1 {
      tint=#4b5563ff
      size=[40, 1]
    }
    component "physics.Body2D" version=1 {
      type=static
    }
    component "physics.BoxCollider2D" version=1 {
      size=[40, 1]
    }
  }
  node "e1000000000000000000000000000002" name="Player" enabled=true layers=1 {
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
  node "e1000000000000000000000000000003" name="Camera" enabled=true layers=1 {
    component "engine.Camera2D" version=1 {
      zoom=36
    }
    component "camera.VirtualCamera2D" version=1 {
    }
    component "camera.CameraFollow2D" version=1 {
      target=node("e1000000000000000000000000000002")
    }
  }
}
```

## Shake the camera on a hard landing

`Landed` passes the speed of the fall. Shake the camera only when it is high, so ordinary hops stay calm. The camera needs a **Camera Shake 2D**:
```aes check
package game

import (
	"aego/camera"
	"aego/input"
)

script Player {
	@export
	var HardLanding float = 12

	func Ready() {
		node.Get<Motion2D>().Landed.Connect(OnLanded)
	}

	func OnLanded(speed float) {
		if speed > HardLanding {
			camera.Shake(0.4)
		}
	}

	func Update(dt float) {
		m := node.Get<Motion2D>()
		m.Move(input.Axis("Move"))
		if input.Released("Jump") {
			m.JumpReleased()
		}
		if input.Pressed("Jump") {
			m.Jump()
		}
	}
}
```

Shake adds trauma from 0 to 1; Camera Shake 2D turns it into movement and lets it decay. Players can scale or turn off shake in the accessibility settings.

```aescene check
scene version=1 {
  node "e1000000000000000000000000000001" name="Floor" enabled=true layers=1 {
    transform {
      position=[0, 3]
    }
    component "engine.SpriteRenderer" version=1 {
      tint=#4b5563ff
      size=[40, 1]
    }
    component "physics.Body2D" version=1 {
      type=static
    }
    component "physics.BoxCollider2D" version=1 {
      size=[40, 1]
    }
  }
  node "e1000000000000000000000000000002" name="Player" enabled=true layers=1 {
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
  node "e1000000000000000000000000000003" name="Camera" enabled=true layers=1 {
    component "engine.Camera2D" version=1 {
      zoom=36
    }
    component "camera.VirtualCamera2D" version=1 {
    }
    component "camera.CameraFollow2D" version=1 {
      target=node("e1000000000000000000000000000002")
    }
    component "camera.CameraShake2D" version=1 {
    }
  }
}
```

## Stomp an enemy

Jumping on an enemy's head is a hitbox on the player's feet that is active only while falling, and a hurtbox on the enemy. The bounce comes from the hitbox's `Hit` signal:
```aes check
package game

import "aego/input"

script Player {
	@export
	var Bounce float = 9

	func Ready() {
		node.Get<Hitbox2D>().Hit.Connect(OnHit)
	}

	func OnHit(target NodeRef, damage int) {
		m := node.Get<Motion2D>()
		m.AddVelocity(Vec2{0, -m.Velocity.Y - Bounce})
	}

	func FixedUpdate(dt float) {
		m := node.Get<Motion2D>()
		node.Get<Hitbox2D>().Active = !m.Grounded && m.Velocity.Y > 0
	}

	func Update(dt float) {
		m := node.Get<Motion2D>()
		m.Move(input.Axis("Move"))
		if input.Released("Jump") {
			m.JumpReleased()
		}
		if input.Pressed("Jump") {
			m.Jump()
		}
	}
}
```

```aescene check
scene version=1 {
  node "e1000000000000000000000000000001" name="Floor" enabled=true layers=1 {
    transform {
      position=[0, 3]
    }
    component "engine.SpriteRenderer" version=1 {
      tint=#4b5563ff
      size=[40, 1]
    }
    component "physics.Body2D" version=1 {
      type=static
    }
    component "physics.BoxCollider2D" version=1 {
      size=[40, 1]
    }
  }
  node "e1000000000000000000000000000006" name="Enemy" enabled=true layers=1 {
    transform {
      position=[4, 2.1]
    }
    component "engine.SpriteRenderer" version=1 {
      tint=#ef4444ff
      size=[0.8, 0.8]
    }
    component "gameplay.Health" version=1 {
      max=1
      team=1
      destroyOnDeath=true
    }
    component "gameplay.Hurtbox2D" version=1 {
      area=[-0.4, -0.4, 0.8, 0.8]
    }
  }
  node "e1000000000000000000000000000002" name="Player" enabled=true layers=1 {
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
    component "gameplay.Hitbox2D" version=1 {
      area=[-0.25, 0.35, 0.5, 0.25]
      damage=1
      active=false
    }
    component "engine.Script" version=1 {
      script "game.game.Player" enabled=true {
      }
    }
  }
  node "e1000000000000000000000000000003" name="Camera" enabled=true layers=1 {
    component "engine.Camera2D" version=1 {
      zoom=36
    }
    component "camera.VirtualCamera2D" version=1 {
    }
    component "camera.CameraFollow2D" version=1 {
      target=node("e1000000000000000000000000000002")
    }
  }
}
```

The player's Health puts it on team 0 and the enemy's on team 1, so the feet hitbox, which takes the team of the nearest Health above it, hurts the enemy and never the player. `examples/platformer` uses the same setup.

## See also

- [Gameplay kit](ref:gameplay)
- [Motion 2D](ref:gameplay.Motion2D)
- [Input](ref:input)
