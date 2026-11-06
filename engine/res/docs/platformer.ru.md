---
id: platformer
title: Платформер с нуля
---
# Платформер с нуля

Это руководство собирает небольшой платформер в пустом 2D-проекте: персонажа, который бегает и прыгает, камеру, которая следует за ним, яму с контрольной точкой, монеты и врага. Каждый шаг заканчивается файлом сцены в том виде, какой он имеет после шага, и каждую сцену и каждый скрипт на этой странице запускают проверки документации.

Если вы ещё не открывали редактор, начните с раздела [Первый проект](ref:quickstart).

Готовый пример — отдельный, более подробный проект `examples/platformer`. Откройте его командой `aego edit examples/platformer` и загрузите основную сцену, чтобы изучить игру из демо на сайте. A/D или стрелки двигают персонажа, пробел — прыжок, J — атака, Esc открывает настройки. F6 включает и выключает освещение, F7 переключает его режимы отладки.

Руководство ниже начинается с простых фигур, а не повторяет все ассеты готового проекта. Его освещение, карты поверхностей, частицы и постобработка описаны отдельно: [Свет и эффекты платформера](ref:platformer-effects).

## 1. Пол и игрок

Создайте проект из шаблона **2D**. В «Иерархии» создайте пустой узел и назовите его **Floor**. Добавьте ему в «Инспекторе» такие компоненты:

- **«Отрисовка спрайта»**, «Размер» `30, 1`, серый «Оттенок»;
- **«Тело 2D»**, «Тип» `static`;
- **«Коллайдер-прямоугольник 2D»**, «Размер» `30, 1`.

Переместите его в `0, 3`. Создайте второй узел, `Player`, с компонентами:

- **«Отрисовка спрайта»**, «Размер» `0.6, 1`, оранжевый «Оттенок»;
- **«Тело 2D»**, «Тип» `kinematic`, «Без вращения» включено;
- **«Коллайдер-прямоугольник 2D»**, «Размер» `0.6, 1`, «Трение» `0`;
- **«Движение 2D»**, «Режим» `platformer`.

«Движение 2D» двигает кинематическое тело: оно скользит вдоль стен, стоит на полу и забирается на невысокие ступеньки. Нажмите «Играть»: игрок падает и приземляется на пол.

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

## 2. Бег и прыжок

Создайте скрипт `Player` в `scripts/game` и добавьте его игроку:

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

`Move` и `Jump` — действия ввода шаблона 2D: стрелки или A и D и пробел, либо геймпад. `Move` принимает значение от -1 до 1, и «Движение 2D» разгоняется до этой доли своей «Макс. скорости». `JumpReleased` обрывает прыжок, если кнопку отпустили рано, поэтому короткое нажатие даёт маленький подскок. «Высота прыжка», «Макс. скорость» и «Время койота» — поля компонента [«Движение 2D»](ref:gameplay.Motion2D).

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

Сцена называет скрипт по модулю, пакету и имени. Здесь модуль — `game`; в вашем проекте это модуль из `project.aego`.

## 3. Камера, которая следует за игроком

Выберите камеру и добавьте **«Виртуальная камера 2D»** и **«Слежение камеры 2D»**. Перетащите игрока из «Иерархии» в поле «Цель» компонента «Слежение камеры 2D». Теперь камера держит игрока в кадре и плавно догоняет его; «Мёртвая зона» и «Демпфирование» задают, насколько свободно. Зоны, тряска и переходы описаны в разделе [Камеры](ref:camera).

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

## 4. Яма и контрольная точка

Разделите пол на две части с промежутком между ними. Под промежутком добавьте узел `Pit` с компонентом **«Смертельная зона 2D»**, «Область» которой накрывает низ уровня. Добавьте игроку **«Здоровье»** и **«Возрождение»**. За ямой добавьте узел `Checkpoint` с компонентом **«Контрольная точка 2D»**.

Падение в яму убивает игрока. Через «Задержку» компонента «Возрождение» он возвращается к последней контрольной точке, которой коснулся, или туда, откуда начал, с полным здоровьем.

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

## 5. Монеты

Монета — это узел с компонентом **«Подбираемый предмет 2D»**. Её подбирают, когда узел с «Движением 2D» входит в её «Область», и после этого она удаляется. Чтобы считать монеты, добавьте игроку счётчик:

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

и дайте каждой монете скрипт, который сообщает об этом игроку:

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

Сделайте одну монету, затем превратите её в префаб (**Создать префаб** в меню «Иерархии») и расставьте её копии; изменение префаба доходит до каждой копии. См. [Префабы](ref:prefabs).

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

## 6. Враг

Враг ходит туда-обратно и ранит игрока при касании. Дайте ему такое же тело, как у игрока, **«Движение 2D»** и **«Хитбокс 2D»** с «Командой» `1`, чтобы он никогда не ранил других врагов из команды 1. Дайте игроку **«Хёртбокс 2D»**: хитбокс наносит урон «Здоровью» узла, хёртбокса которого касается. После этого игрок мигает в течение «Времени неуязвимости», и его нельзя ударить снова, пока оно не закончится.

Враг разворачивается у стен:

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

`HitWall` даёт нормаль стены, направленную от стены, поэтому её X — новое направление.

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

## 7. Играйте где угодно

**Файл → Экспорт…** собирает игру для Linux, Windows, macOS или веба, как в разделе [Первый проект](ref:quickstart). На телефоне веб-сборка показывает сенсорный геймпад: стик двигает, а A прыгает, потому что действия привязаны и к геймпаду.

## Что дальше

- [Рецепты](ref:recipes): двойной прыжок, односторонние платформы, движущиеся платформы, рывок и многое другое.
- [Набор геймплея](ref:gameplay): все компоненты геймплея в одном месте.
- [Анимация](ref:animation): анимация спрайтов и машины состояний для игрока.
- [Свет и эффекты платформера](ref:platformer-effects): карты поверхностей, источники света, частицы и постобработка готового примера.
- `examples/platformer` в исходниках движка — эта же игра, развитая дальше: анимация, звук, прыжки на врагов, сохранения, настройки и 2D-освещение.
