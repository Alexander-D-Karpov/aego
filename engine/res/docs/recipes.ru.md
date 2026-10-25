---
id: recipes
title: Рецепты
---
# Рецепты

Короткие ответы на частые вопросы о платформерах. Каждый рецепт — это скрипт и сцена, в которой он работает, на основе игрока из руководства [Платформер с нуля](ref:platformer); каждый из них запускается проверками документации.

## Двойной прыжок

«Движение 2D» прыгает только с земли или в пределах «Времени койота». Второй прыжок в воздухе задаёт скорость вверх напрямую, а счётчик сбрасывается при приземлении:
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

Скорость задаётся в единицах в секунду, а вверх — это отрицательный Y, поэтому добавленная скорость гасит текущую вертикальную скорость и задаёт `AirJumpSpeed` вверх.

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

## Односторонние платформы

Платформе, через которую игрок запрыгивает снизу и на которой стоит сверху, скрипт не нужен. Дайте ей **«Коллайдер-линию 2D»** с двумя точками и включённым флагом «Односторонний». Скрипт игрока — обычный, из руководства:

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

## Движущиеся платформы

Кинематическое тело, которое двигается через `MoveTo`, везёт стоящее на нём «Движение 2D», пока включены «Движущиеся платформы»; по умолчанию они включены. Этот скрипт двигает своё тело туда и обратно между начальной точкой и `Travel`:
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

«Платформа при уходе» в «Движении 2D» игрока определяет, что происходит со скоростью платформы, когда игрок с неё спрыгивает.

## Рывок

`Dash` двигает с постоянной скоростью в течение заданного времени и всё это время не учитывает ввод и гравитацию, если не включена «Гравитация в рывке». Перезарядка не даёт повторять рывок без остановки:
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

Здесь клавиша читается напрямую. В игре добавьте действие `Dash` в **Настройках проекта → Ввод** и читайте его через `input.Pressed("Dash")`, чтобы его можно было переназначить и чтобы оно работало на геймпаде.

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

## Тряска камеры при жёстком приземлении

`Landed` передаёт скорость падения. Трясите камеру, только когда эта скорость велика, чтобы обычные подскоки оставались спокойными. Камере нужна **«Тряска камеры 2D»**:
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

Тряска добавляет «травму» от 0 до 1; «Тряска камеры 2D» превращает её в движение и даёт ей затухнуть. Игроки могут ослабить или выключить тряску в настройках доступности.

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

## Прыжок на врага

Прыжок на голову врага — это хитбокс на ногах игрока, активный только во время падения, и хёртбокс на враге. Отскок даёт сигнал `Hit` хитбокса:
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

«Здоровье» игрока ставит его в команду 0, а «Здоровье» врага — в команду 1. Хитбокс на ногах берёт команду ближайшего «Здоровья» над ним, поэтому ранит врага и никогда не ранит игрока. `examples/platformer` использует ту же схему.

## См. также

- [Набор для геймплея](ref:gameplay)
- [Движение 2D](ref:gameplay.Motion2D)
- [Ввод](ref:input)
