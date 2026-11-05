---
id: platformer
title: Платформер с нуля
---
# Платформер с нуля

Это руководство собирает небольшой платформер в пустом 2D-проекте: персонажа, который бегает и прыгает, камеру, которая следует за ним, яму с контрольной точкой, монеты и врага. Каждый шаг заканчивается файлом сцены в том виде, какой он имеет после шага, и каждую сцену и каждый скрипт на этой странице запускают проверки документации.

Если вы ещё не открывали редактор, начните с раздела [Первый проект](ref:quickstart).

Готовый пример — отдельный, более подробный проект `examples/platformer`. Откройте его командой `aego edit examples/platformer` и загрузите основную сцену, чтобы изучить игру из демо на сайте. A/D или стрелки двигают персонажа, пробел — прыжок, J — атака, Esc открывает настройки. F6 включает и выключает освещение, F7 переключает его режимы отладки.

В иерархии изучите эмиттеры Dust и Sparks игрока, а также Smoke и Embers у StartLantern. События приземления и атаки запускают всплески частиц. Bloom на камере создаёт ореол на экране, а Light2D фонаря освещает соседние поверхности Lit; контуры Light Occluder 2D отбрасывают тени. Редактируемые примеры и сцены Try It находятся в разделах [частицы](ref:particles), [материалы и свет](ref:render2d/materials) и [постобработка](ref:render2d.PostProcess). Руководство ниже начинается с простых фигур, а не повторяет все ассеты готового проекта.

## 1. Пол и игрок

Создайте проект из шаблона **2D**. В «Иерархии» создайте пустой узел и назовите его `Floor`. Добавьте ему в «Инспекторе» такие компоненты:

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
- `examples/platformer` в исходниках движка — эта же игра, развитая дальше: анимация, звук, прыжки на врагов, сохранения, настройки и 2D-освещение.

## Освещение и карты поверхностей в примере

Откройте существующий проект `examples/platformer`, чтобы увидеть полный пример освещения. Уровень, коллизии и управление сохраняют игровой сценарий; добавленные карты поверхностей и источники показывают нормали, тени и эмиссию в том же уровне. F6 переключает освещение вида. F7 переключает обычный вид, нормали и тени. Обе клавиши зарегистрированы как действия ввода и не меняют материалы или состояние симуляции.

| Ассет проекта | Связь с картой поверхности |
|---|---|
| `assets/surfaces/platform.png` | Явный источник высоты — `platform_height.png`, у которого включена генерация нормалей при импорте. Полученная normal map общая для платформ уровня. |
| `ground.png` и `wall.png` | Явные normal maps имеют приоритет; исходные карты высоты также связаны с ними для авторинга. |
| `runner.png` | У каждого из восьми исходных кадров анимации есть собственная стабильная связь с normal-кадром в `runner_n.png`. Исходный силуэт, размер кадра и игровая точка отсчёта сохранены. |
| `lamp.png` | `lamp_em.png` — маска эмиссии. Она осветляет сердцевину лампы через `lamp_lit.aemat`; соседний Point light освещает другие объекты. Маска не создаёт Bloom. |
| `window_cookie.png` | Линейная cookie-маска задаёт форму Sprite light, не используя координаты атласа альбедо. |

Чтобы изменить поверхность существующей платформы, откройте `platform.png` из Assets и перейдите по **источнику высоты** к `platform_height.png`. Включите **освещённую поверхность** и измените силу высоты в общем документе этой карты. Кандидат normal map и перемещаемый источник предпросмотра показывают результат до сохранения. Включите Play, оставьте вкладку изображения открытой и нажмите **Сохранить настройки**: работающий уровень получит новое подготовленное поколение нормалей. Undo возвращает прежний вид, оставляя документ метаданных изменённым, а сохранённый файл — прежним; Redo возвращает сохранённый результат. Stop возвращает неизменённую EditScene. Явно назначенная Normal-карта имела бы приоритет над нормалью, полученной из Height.

Откройте `runner.png`, выберите существующий кадр и используйте Flip X/Y предпросмотра, чтобы проверить связанную normal map. Эти переключатели действуют только на предпросмотр; игровой скрипт сам задаёт FlipX при смене направления. Пиксели исходного изображения редактируются во внешнем графическом редакторе, а связи каналов и настройки импорта — в общем документе ассета с его историей Undo.

У игрока и врага для теней заданы отдельные полигоны `LightOccluder2D`. Они приблизительно повторяют голову и ноги общего спрайта, не меняя капсулу игрока и прямоугольный коллайдер врага. Контуры статичны и не подстраиваются под отдельные кадры анимации.

Правила ролей и цветового пространства, диагностика отсутствующих ссылок и модель освещения описаны в разделах [Просмотр изображений и спрайтов](ref:render2d/image-viewer) и [Материалы и шейдеры](ref:render2d/materials). Native- и web-экспорты используют этот же проект и включают нормали, результат преобразования высоты, эмиссию и cookie. Демо сайта подготавливается из этого экспорта; подготовка сама по себе не публикует новую версию сайта.

## Пыль и искры в примере

Откройте `examples/platformer`, прыгните и после приземления продолжайте бежать. Пыль появляется у ног персонажа и остаётся над местом, где она возникла. Чтобы настроить эффект, выберите `Player/Dust`: `rectSize` задаёт размеры узкой полосы под ногами, а `lifetime` — время жизни пылинки. В `scripts/player/player.aes` обработчик `OnLanded` выпускает восемь частиц, `OnJumped` — пять, а `OnClimbed` — шесть. У эмиттера выключен `playOnStart`, а `rate` равен нулю, поэтому частицы появляются только при этих событиях. `Player/Slash/Sparks` выпускает искры по событию Hit в анимации атаки.

При `space=world` движение эмиттера не сдвигает уже родившиеся частицы. Выберите `space=local`, если эффект должен двигаться вместе с родителем. Чтобы сравнить этот же уровень с частицами и без них, переключите **Эффекты частиц** в настройках специальных возможностей игры. F6 и F7 управляют освещением; при сравнении оставьте их в одном положении.

У фонаря `StartLantern` выберите дочерний узел `Smoke` или `Embers`: это два независимых источника дыма и искр. В предпросмотре Inspector можно поставить эффект на паузу, закрепить зерно и сравнить изменения до запуска игры. В разделе **По ходу жизни** перетащите ключ градиента, отпустите мышь и нажмите Undo, чтобы восстановить прежний цветовой переход. Пять пресетов и работающий пример собраны в разделе [Эффекты частиц на CPU](ref:particles).

## Постобработка на камере готового платформера

Откройте платформер в Game View и выберите **Camera → Post Process**. В сцене включён Bloom с Threshold 0,75, Intensity 0,22 и Radius 6. Меняйте Intensity и наблюдайте за ореолом вокруг лампы. В стеке также есть выключенные Vignette, Color Grading и пользовательский материал Cold. Включите один из них для сравнения. Во время Play откройте настройки и сравните непрозрачную часть меню при включённом и выключенном компоненте. Чтобы вернуть исходное изображение мира, выключите Post Process. F6 и F7 управляют освещением и его диагностикой.

Центр задаётся относительно выхода камеры: (0.5, 0.5) — середина. Интенсивность задаёт степень затемнения, мягкость — ширину перехода. Радиус измеряется относительно короткой стороны изображения: на широком экране окружность не превращается в овал.
