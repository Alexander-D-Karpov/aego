---
id: language/script-example
title: Пример анимированного скрипта
---
# Пример анимированного скрипта

Исходный пример вращается и подпрыгивает с помощью `Ready` и `Update`. Его экспортированные поля `Speed`, `Bob` и `Rate` можно изменить в Инспекторе.

Запустите небольшой пример, который перемещает свой узел при начале игры:

```aes run
package demo

script MoveOnReady {
    func Ready() {
        node.SetPosition(Vec2{2, 0})
    }
}
```
