---
id: particles
try: try/ParticleSystem2D.aescene
try_select: particles.ParticleSystem2D
icon: comp.sprite
group: gameplay
title: Двумерные частицы
order: 8
---
# Двумерные частицы

В меню создания выберите **Effects → Dust** для ручного импульса или **Smoke** для постоянной эмиссии. Для ручного Burst допустим Rate=0. В World пыль остаётся на полу после ухода игрока, в Local существующие частицы следуют за эмиттером. Stop выключает рождение, Clear удаляет живые частицы.

В платформере обработчик Landed вызывает Burst(8) у дочерней Dust. У StartLantern независимые Smoke и Embers. В примере topdown пыль появляется от событий Footstep существующего клипа бега.

У эмиттера собственный seed. Отключение эффектов не меняет физику или игровой RNG. При Reduced Motion действует выбранная автором политика.

В нативной Go-системе `sdk.ParticlesOf(ctx)` возвращает сервис частиц данного мира. Вызовите на нём `fx.Burst(dustNode, 8)`. Если модуль не подключён, сервис равен nil.
