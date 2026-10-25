---
id: settings
title: Настройки
---
# Настройки

Один реестр хранит все параметры, которые может менять игрок или проект: режим окна, громкость, привязки клавиш, специальные возможности. Каждая настройка принадлежит модулю и имеет id вида `<module>.<path>`, например `display.window.mode` или `audio.bus.music.volume`.

## Откуда берётся значение

| Слой | Кто задаёт | Где хранится |
|---|---|---|
| по умолчанию | модуль | код |
| проект | **Настройки проекта → По умолчанию** | `project.aego`, блок `defaults {}` |
| пользователь | игрок, экран настроек, скрипты | профиль игрока |
| сеанс | `settings.Override*` | не сохраняется |

Действует последний слой, в котором есть значение. Катсцена может приглушить музыку через `OverrideFloat`, а потом снять это через `ClearOverride`; выбор игрока остаётся нетронутым.

## Профиль игрока

`<user config>/aego/<module>/profiles/<name>/settings.aetext`; `aego run --profile test` и `aego edit --profile test` используют другой профиль. Игра в редакторе делит профиль с собранной игрой. Повреждённый файл сохраняется как `settings.aetext.bad`, и игра запускается со значениями по умолчанию. Значения модулей, которые игра не загружает, хранятся без изменений.

## Экран настроек

Его открывает F10 во время игры и в `aego run` или `settings.OpenView()` из скрипта; пока он открыт, игра стоит на паузе. «Применить» задаёт все изменённые значения разом: если одно применить нельзя, не меняется ни одно. Режим окна, монитор и разрешение просят подтверждения и возвращаются назад через 15 секунд без него.

`display.renderScale` рисует игру во внеэкранное изображение размером «окно × масштаб» и растягивает его на окно в `aego run`; ниже 1 он меняет чёткость на скорость, выше 1 даёт суперсэмплинг. Экран настроек рисуется в полном разрешении поверх этого изображения. Панель «Игра» в редакторе всегда рисует с масштабом 1.

## Скрипты

    import "aego/settings"

    shake := Strength * settings.GetFloat("accessibility.screenShake")
    settings.SetFloat("audio.bus.music.volume", 0.5)
    settings.Apply()

| Функция | |
|---|---|
| `GetBool GetInt GetFloat GetString(id)` | действующее значение |
| `SetBool SetInt SetFloat SetString(id, v)`, `Reset(id)`, `Apply() bool`, `Revert()` | подготовленные изменения |
| `OverrideBool OverrideInt OverrideFloat OverrideString(id, v)`, `ClearOverride(id)`, `ClearOverrides()` | значения сеанса |
| `OpenView() CloseView() ViewOpen()`, `NeedsRestart()`, `Save()` | экран и профиль |

Скрипт реагирует на изменения методом-событием:

    func OnSettingChanged(id string) {
      if id == "accessibility.reducedMotion" {
        reduced = settings.GetBool(id)
      }
    }

Он вызывается по разу на каждую изменённую настройку после «Применить», «Отменить», переопределения или истечения времени подтверждения; для изменений из скриптов — в том же кадре.

## Go

    sdk.Setting(r, sdk.SettingDesc{
      ID: "mygame.difficulty", Kind: schema.KindEnum, Category: "Gameplay",
      Flags: sdk.SettingUser | sdk.SettingProject,
      Choices: []sdk.SettingChoice{{Name: "easy", Value: schema.Enum(0)}, {Name: "hard", Value: schema.Enum(1)}},
    })
    st := sdk.Settings(ctx)
    k := st.Key("mygame.difficulty")
    hard := st.Int(k) == 1

В коде, который выполняется каждый кадр, читайте через `Key`; это не выделяет память.

## Правила для модулей

- Тряска камеры умножает свою силу на `accessibility.screenShake`.
- Когда включён `accessibility.reducedMotion`, камеры, частицы и переходы урезают крупное или быстрое движение.
- Параметр качества регистрирует `Presets` для каждого имени пресета; `display.preset` задаёт их, а ручное изменение переключает пресет на «Своё».
