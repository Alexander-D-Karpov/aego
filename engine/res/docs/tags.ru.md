---
id: tags
title: Теги и запросы
---
# Теги и запросы

![Компонент «Теги» в Инспекторе](shots/tags.png)

Теги группируют узлы по смыслу: `Enemy`, `Enemy.Flying`, `Pickup.Coin`. Добавьте компонент «Теги» в Инспекторе или вызовите `node.AddTag` из скрипта; ищите узлы через `scene.WithTag`.

---
id: tags/names
title: Имена тегов
---
# Имена тегов

Тег — это сегменты из букв, цифр и `_`, соединённые точками. Узел с тегом `Enemy.Flying` находится и по `Enemy`, поэтому для широкой и узкой группы нужен один тег. Теги различают регистр. Неправильный тег игнорируется, а «Проблемы» показывают TAG001; тег, указанный дважды, показывает TAG002.

---
id: tags/scripts
title: В скриптах
---
# В скриптах

    var enemies []NodeRef

    func Ready() {
      for i := 0; i < 64; i++ {
        enemies = append(enemies, nil)
      }
    }

    func Update(dt float) {
      n := scene.WithTag("Enemy", enemies)
      for i := 0; i < n; i++ {
        debug.Circle(enemies[i].WorldPosition(), 0.5, Color{1, 0, 0, 1})
      }
    }

| Функция | |
|---|---|
| `node.HasTag(t)`, `node.AddTag(t)`, `node.RemoveTag(t)` | теги одного узла |
| `scene.WithTag(tag, out) int` | включённые узлы с тегом, в порядке, в котором они его получили |
| `scene.WithComponent(name, out) int` | включённые узлы с компонентом (`"Body2D"` или полное имя) |
| `scene.InArea(rect, mask, out) int` | тела, касающиеся прямоугольника на слоях из `mask`, когда работает физика; иначе включённые 2D-узлы, позиция которых лежит в нём |
| `scene.ByID(id) NodeRef` | узел с id из файла сцены |

Запросы пишут не больше `len(out)` узлов и ничего не выделяют; задайте размер среза один раз. Они видят включённость узлов по последнему проходу трансформаций. Без физики узел — точка: ядро не знает размера спрайта или меша, и `InArea` не учитывает `mask`.

---
id: tags/go
title: В Go
---
# В Go

    s.AddTag(h, "Enemy.Flying")
    n := s.WithTag("Enemy", buf)

`WithTagAll` включает выключенные узлы; `WithComponent(typeID, out)` и `InRect(rect, out)` — другие запросы. Когда код на Go меняет `Tags.Tags` напрямую, вызовите `s.TagsChanged(h)`, чтобы индекс это увидел.
