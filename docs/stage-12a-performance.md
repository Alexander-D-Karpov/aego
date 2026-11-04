# Stage 12a — performance report

## Что измерено

Сравнение выполнено с присланным исходным снимком `ad23798da5abce89e850ceb9b312c2834daadc95`. Для обоих деревьев использован один и тот же исходник `tools/materialbench/main.go`; зависимости `aego/*` разрешались относительно соответствующего дерева. Это сравнение подготовки и отправки draw stream на CPU через `gpu/null`, а не измерение FPS видеокарты.

Окружение: Go 1.27.0 linux/amd64, AMD EPYC 9V74, CPU affinity 2, `GOMAXPROCS=1`; build flags `-trimpath -buildvcs=false`. В каждом сценарии 3000 спрайтов, viewport 1280×720, DPR1, фиксированные позиции/tint/input, 120 прогревочных кадров. Пять выборок по 1000 кадров; allocations измерялись на 200 прогретых кадрах каждой выборки. Сначала завершён before, затем after, без параллельных builds/tests/browser runs команды. Результат — медиана пяти выборок; полные выборки сохранены в evidence JSON.

## Steady-state CPU и GPU submissions

| Сценарий | Before, µs/кадр | After, µs/кадр | Изменение | Draws / batches | Buffer bytes/кадр | Uploads/кадр | Go allocations/кадр |
|---|---:|---:|---:|---:|---:|---:|---:|
| Default | 202.243 | 147.190 | −27.22% | 1 / 1 | 156128 | 3 | 0 |
| Один material | 184.461 | 163.325 | −11.46% | 1 / 1 | 156128 | 3 | 0 |
| Разные numeric overrides | 206.950 | 176.427 | −14.75% | 1 / 1 | 348128 | 4 | 0 |
| Повторяющийся A/B/A | 311.611 | 272.689 | −12.49% | 2001 / 2001 | 156128 | 3 | 0 |
| Две камеры | 393.333 | 318.745 | −18.96% | 2 / 2 | 312240 | 5 | 0 |

Draws, batches, instance bytes, buffer bytes, uploads и allocations одинаковы до/после во всех выборках. Для default/one-material/A/B/A instance stream равен 156000 bytes; overrides добавляют 192000 bytes typed lanes; две камеры отправляют 312000 instance bytes. Материалы A/B/A сохраняют painter order: при 3000 спрайтов получается 2001 совместимая соседняя группа, а не сортировка всех A вместе. Первый shader compile и initial material UBO upload не входят в steady-state.

Trim использует дополнительный поток только для затронутых batches; обычный instance остался 52 bytes. Отдельный `TestTrimmedMaterialSteadyStateDoesNotAllocate` проверяет trim + typed lanes: один batch и 0 Go allocations после прогрева. Эти дополнительные bytes не спрятаны внутри сравнения обычного нетриммированного случая.

### Найденная и исправленная регрессия

Первая сопоставимая пара после функциональных изменений показала ухудшение CPU: default +21.73%, один material +16.22%, overrides +9.19%, A/B/A +9.53%, две камеры +16.27%. Исходные JSON сохранены рядом с финальными. Профиль указал на ненужные sin/cos при нулевом rotation, обработку отсутствующих trim/order/mesh streams и повторные map writes. Исправлены эти быстрые пути; visual order, source сцен, количество sprites, assertions и image references не упрощались. После этого выполнена финальная пара из таблицы и заново пройдены pure, native и browser material проверки. Отдельный шумный запуск с overlap помечен discarded и не использован для медиан.

## Три release wasm-профиля

Одинаковые before/after flags: `GOOS=js GOARCH=wasm go build -trimpath -ldflags="-s -w"`, runtime tag `aego_runtime`, try tag `aego_editor`; `GOFLAGS=-buildvcs=false`. Gzip посчитан одинаковым Python `gzip.compress(..., compresslevel=9, mtime=0)`. Это сопоставимые profile binaries; экспорт конкретного проекта может иметь другой module set и gzip encoder.

| Профиль | Wasm before | Wasm after | Gzip before | Gzip after | Gzip delta | Прежний gzip budget | Запас |
|---|---:|---:|---:|---:|---:|---:|---:|
| runtime | 25999660 | 27004085 | 6196959 | 6377087 | +180128 / +2.91% | 8388608 | 2011521 |
| playground | 27884395 | 28974315 | 6647066 | 6851905 | +204839 / +3.08% | 8388608 | 1536703 |
| try/docs | 30120903 | 31186949 | 7156543 | 7364813 | +208270 / +2.91% | 10485760 | 3120947 |

Лимиты 8/8/10 MiB не изменены. `wasmDenied`, исключение native self-update stack и независимые dependency checks сохранены. Итоговый CI дополнительно проверяет размеры файлов, созданных штатным exporter/docs build.

## Driver, reload и браузеры

GL33 выполняется через SDL 3.2.24 offscreen/EGL и Mesa llvmpipe; WebGL2 — Chromium 141 / ANGLE SwiftShader и Firefox 142 / Mesa. Это настоящие API compile/link/readback, но программные GPU. Native material image tests и два браузерных harness проверяют bounded resource counts при reload; реальные loss/restore события проверены во время ещё не принятого shader candidate. Подробные результаты и ограничения перечислены в `stage-12a-completion.md`.

Отдельный `TestMaterialPhaseMeasurements` измеряет пять import/reference, decoder и driver-link выборок, initial render/readback prewarm, enqueue и frame-boundary publication. Driver cache намеренно не объявляется cold: он не очищается. Frame-boundary время включает compile/link, layout validation, migration и commit; это не чистое время присваивания указателя. Каждая публикация сохраняет explicit value и одинаковое количество GPU objects. Финальный проход на llvmpipe дал следующие наблюдения (медианы 5 выборок):

| Фаза | Wall time, ms |
|---|---:|
| Import + reference link GLSL330/ESSL300 | 79.733822 |
| AESH decode → CPU definition | 0.028265 |
| Отдельный driver compile/link | 3.417261 |
| Queue candidate | 0.005869 |
| От начала frame boundary до commit callback | 0.575122 |
| Весь frame boundary, включая retirement | 0.602345 |

Первый material draw с readback занял 64.947265 ms; driver shaders к этому моменту уже были отдельно linked, поэтому это не cold-driver benchmark. После 5 reload generations количества совпали: 9 buffers, 3 textures, 1 sampler, 5 shaders, 15 pipelines, 0 pending compiles. Эти after-only наблюдения снимались во время общего CI и не используются как воспроизводимый performance regression threshold. Полные samples и обозначение границ фаз сохранены в `evidence/aego-material-phases.json`.

Физическая reference GPU недоступна в этом окружении; CPU/null и software-GPU browser observations имеют разную область применимости. Парный browser-прогон описан ниже.

## Реальные browser-профили: before/after

Браузерный пробел закрыт отдельным парным прогоном **2026-10-30 17:34:11–17:35:16 UTC**. Baseline и final собраны отдельно; до/после сборки baseline сверены SHA256 2931 исходного файла, изменений нет. Браузер: Chromium 141.0.7390.37 / ANGLE SwiftShader, CPU AMD EPYC 9V74. Каждый профиль запускается before → after; процессы браузера выполняются последовательно. Во время пары команды не запускали других builds/tests/GPU jobs. Final race завершился до пары; общий CI запущен после неё.

Одинаковы build mode, viewport, DPR, scene/input и число измеряемых кадров. Runtime: 1280×720, DPR 1, seed 1 и один `platformer_run.aeinput`, 120 прогревочных + 600 измеряемых кадров, lockstep. Playground: 1280×800, DPR 1, 1000 animated sprites, 120 прогревочных + 120 измеряемых engine frames. Try: 1280×900, DPR 1, `physics.Body2D` с настоящим Inspector, 120 + 120 frames; Box имеет одинаковую позу до/после и не движется внутри каждого окна. Материал добавляет реальную строку Inspector, поэтому состав UI у before и after закономерно отличается.

**Один парный timing run на профиль не является статистическим доказательством ускорения или отсутствия регрессий.** Значения показывают наблюдение на software GPU. Runtime использует bridge counters движка; Playground/Try — один и тот же JS WebGL2 probe. Его overhead и host scheduling входят в callback time. API draws включают presentation; queries показаны отдельно, их число зависит и от задержки GPU completion.

| Профиль | Измеряемых кадров | Calls/кадр, before → after | Queries/кадр | API draws/кадр | Uploads/кадр | Buffer bytes/кадр | Frame interval p50/p95, ms | Callback work p50/p95, ms |
|---|---:|---:|---:|---:|---:|---:|---:|---:|
| runtime | 600 | 57.017 → 60.017 | 5.932 → 5.988 | 3.867 → 3.867 | 3 → 3 | 555.7 → 555.7 | 16.70/16.80 → 16.70/16.80 | 1.30/2.70 → 1.40/2.70 |
| playground | 120 | 31.000 → 34.000 | 6.808 → 6.592 | 2.000 → 2.000 | 3 → 3 | 52128.0 → 52128.0 | 16.70/33.40 → 16.70/33.40 | 1.30/1.70 → 1.40/1.90 |
| try | 120 | 823.000 → 847.000 | 6.433 → 6.417 | 89.000 → 91.000 | 7 → 8 | 33836.0 → 33020.0 | 33.30/50.10 → 33.30/49.90 | 3.00/6.40 → 3.00/8.90 |

У runtime и playground сохранены draws/uploads/bytes; регулярных WebGL calls стало на 3 больше за кадр. Причина сверена с кодом: `Renderer.View` передаёт непустой `ClearRect`, а WebGL2 clear выполняет Enable(SCISSOR_TEST), Scissor и Disable(SCISSOR_TEST). Сейчас этот путь применяется и к full-target viewport; это стоимость корректного camera-scoped clear. После измерения дополнительная оптимизация не вносилась. Разница показана явно, а не скрыта усреднением с CPU/null benchmark. В Try callbacks p95 вырос с **6.4 до 8.9 ms** при одинаковом p50 3.0 ms. Frame interval p95 в этой паре — 50.1 → 49.9 ms; это не отменяет наблюдаемое увеличение callback tail. Окна 120 кадров Playground заняли 2497.3 → 2772.8 ms, Try — 3770.7 → 3543.9 ms. Эти единичные wall-time наблюдения не используются как causal/statistical performance gate.

Runtime process-wide allocations в этой паре: 4.7333 → 4.335 на кадр; это весь Go runtime с probe/background work. Отдельные `Render`, `RenderPrepare`, `RenderUI` и `gpu` zones имеют 0 в обеих версиях. Значения не подменяют измеренные выше 0 Go allocations в собственном прогретом material/render пути.

Полные raw reports и logs шести запусков лежат в `evidence/browser-pair/{before,after}/`. Manifest содержит точные команды, времена, SHA256 обоих wasm, replay и probes. Legacy поле `traffic.method` у JS probe содержит слово `after-only` из первой версии измерителя; фазу этого парного запуска задают outer `before`/`after` пути и manifest. Это только устаревшее описание метода, а не смена измеренных данных. Reusable runner — `tools/websmoke/profile-pair.mjs`; он явно отделяет measurement mode от functional/allocation gates и отвергает совместный `--gate`.

### Штатные site/export размеры и отдельные gates

У штатной site/export сборки другой module set и Go gzip encoder; её размеры следует сравнивать между собой, отдельно от Python gzip-таблицы выше:

| Профиль | Wasm before → after | Gzip before → after | Прежний gzip budget | Результат after |
|---|---:|---:|---:|---|
| runtime / platformer | 26085733 → 27090597 | 6272342 → 6459389 | 8388608 | PASS |
| playground | 27884395 → 28974315 | 6708846 → 6921565 | 8388608 | PASS |
| try/docs | 30120903 → 31186949 | 7231188 → 7437053 | 10485760 | PASS |

Functional и allocation gates выполняются отдельно от measurement mode. Runtime allocation gate с прежним allowlist: 23 разрешённых first-use/high-water allocations за 600 кадров и **0 неразрешённых**. Найденные 20 лишних allocations происходили из speculative error при разрешении корректного `SpriteRenderer.tint.a`; resolver теперь проверяет наличие полного component prefix до создания ошибки, сохраняя precedence и diagnostics неверных путей.

Playground smoke после настоящего context restore сохранил прежние image references (различия 0.005% и 0.058%). Исправлена только граница capture при дробном CSS origin. Try действительно выполняет keyboard movement до проверки изображения; затем reset/pause за одну JS task даёт проверенное начальное состояние с нулём simulation frames. Одно новое изображение, включающее Material row, просмотрено вручную; повторный проход даёт 0.000%.

Runtime demo/profile persistence/touch/fullscreen/30 fps/DPR cap, theme switching и последовательный audio gate прошли. У audio messages/shared/runtime demo — 0 underruns; ранняя попытка с 10 underruns сохранена отдельно и не считается PASS. Браузер разрешил AudioContext без жеста, поэтому ветка, которая требует gesture gate, отмечена skipped. Предыдущий after-only отчёт сохранён в `evidence/aego-browser-profiles-after.json` и `.md`; текущая парная таблица основана на отдельных более поздних raw reports, а не на смешивании двух режимов измерения.

## Воспроизведение

Из корня before checkout собрать тот же final benchmark source; затем из корня final checkout собрать его ещё раз:

```sh
GOMAXPROCS=1 go build -trimpath -buildvcs=false \
  -o /tmp/aego-materialbench-before /absolute/final/source/tools/materialbench/main.go

# Следующая команда выполняется из final checkout.
GOMAXPROCS=1 go build -trimpath -buildvcs=false \
  -o /tmp/aego-materialbench-after /absolute/final/source/tools/materialbench/main.go

# Последовательно, в спокойном интервале.
GOMAXPROCS=1 taskset -c 2 /tmp/aego-materialbench-before \
  -sprites 3000 -runs 5 -frames 1000 -out /tmp/aego-material-bench-before.json
GOMAXPROCS=1 taskset -c 2 /tmp/aego-materialbench-after \
  -sprites 3000 -runs 5 -frames 1000 -out /tmp/aego-material-bench-after.json

AEGO_REQUIRE_GLSLANG=1 AEGO_MATERIAL_PHASE_REPORT=/tmp/aego-material-phases.json \
  go test -tags aego_gltest ./tests/gl -run '^TestMaterialPhaseMeasurements$' -count=1 -v
```

Последняя команда требует доступного OpenGL-контекста. В использованном Linux окружении дополнительно задавались `SDL_VIDEODRIVER=offscreen`, `SDL_AUDIO_DRIVER=dummy`, `LIBGL_ALWAYS_SOFTWARE=1`, `GALLIUM_DRIVER=llvmpipe`, `MESA_GL_VERSION_OVERRIDE=3.3`, `MESA_GLSL_VERSION_OVERRIDE=330`.

## Исходные измерения в комплекте

В архиве поставки `evidence/` содержит полные before/after samples, environment, comparison, первую регрессионную пару, wasm sizes, driver phase measurements и browser JSON. Логи с пропущенными или упавшими попытками не выдаются за успешные: финальный completion report отделяет окончательные проходы от диагностических запусков.
