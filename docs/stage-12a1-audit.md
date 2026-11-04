# Stage 12a1 — аудит исходного снимка

Этот отчёт описывает проверенный исходный код, а не предполагаемый объём checkpoint. Итоговая приёмка изменений приведена отдельно в `stage-12a-completion.md`, численные сравнения — в `stage-12a-performance.md`.

## Исходная точка

Пользователь предоставил архив исходников и контекста с HEAD `ad23798da5abce89e850ceb9b312c2834daadc95` от 2026-10-28. В истории указан checkpoint 12a1 `132ddef`. Снимок проверялся отдельно от изменяемого дерева. Синтетический локальный Git commit нужен только для получения patch; он не заменяет пользовательский commit и не публиковался.

В исходном рабочем дереве уже находились изменения `examples/platformer/scenes/main.aescene` и новые `examples/platformer/scripts/platform/platform.aes` и `.meta`. Эти три файла сохранены без изменений. Патч Stage 12a их не содержит. Версия движка, номера будущих Lighting/Particles/PostFX и лимиты wasm не повышались.

В настоящем реестре D365/D366 относятся к 12w. Для материалов применимы D367, D368, D380, D381, D383 и D393: обновлены именно эти решения, без перезаписи занятых номеров. Уже существовавшие корректные std140 layout, declared schema, asset IDs, registry, Mutation Service, propref и экспорт в каталог сохранены.

## Окружение и исходные измерения

- В архиве пользователя: `go1.27.0-X:nodwarf5 linux/amd64`. В проверяющем окружении: официальный Go 1.27.0, SDL 3.2.24, glslang 15.1.0, Linux amd64.
- Native GPU API: GL33 через SDL offscreen/EGL и Mesa llvmpipe. Это реальный compile/link/readback, но программный GPU. Физические GPU в окружении отсутствуют.
- WebGL2: Chromium 141.0.7390.37 / ANGLE SwiftShader и Firefox 142.0.1 / Mesa. Наличие браузера или успешная сборка wasm не считались успешным GPU-тестом.
- Исходные GL goldens выполнялись на том же llvmpipe и прошли. Повторный verbose-прогон подтвердил renderer, выполнение всех сцен `TestGolden` и `TestEditorInspectorMultiLocked`, отсутствие Skip.
- Исходные Go/integration проверки обнаружили `TestReadCommandsGolden`: он копировал живой пример платформера и сравнивал его со старым JSON. Присланные незавершённые правки платформера меняли этот результат ещё до 12a. Итоговый тест использует фиксированный маленький проект с Material/MaterialOverride; остальные mutation/run tests продолжают проверять настоящий пример. Ожидаемые JSON просмотрены, `project.validate` требует нулевые ошибки.
- До исправлений измерены три release wasm-профиля: runtime 25 999 660 / 6 196 959 bytes, playground 27 884 395 / 6 647 066 bytes, try 30 120 903 / 7 156 543 bytes (wasm / gzip). Бюджеты остаются 8 MiB gzip для runtime/playground и 10 MiB для try.
- Общий контрольный поток: 3000 спрайтов, viewport 1280×720, DPR1, одна и две камеры. До исправлений default/one-material/overrides дают один batch, повторяющееся A/B/A — 2001, две камеры — два. Во всех пяти warmed сценариях 0 Go allocations. Полная методика и итоговые числа находятся в performance report.

## Матрица фактической реализации 12a1

Статус в этой таблице относится к **присланному снимку до исправлений**. `Unverified` означает отсутствие доказательства в исходных проверках, а не успешное выполнение.

| Область, исходный файл / символ | Исходный статус | Проверка и выполненная работа |
|---|---|---|
| `render/shaderx/unit.go`: `Parse`, `param`, `reUniform`, `reAnnot` | Broken | Regex metadata не учитывали произвольные comments/scope. Добавлены tokenizer и ограниченный parser; `TestCommentsDoNotDeclareMetadataOrEntries`, `TestSchemaCannotDependOnPreprocessorOrLocalScope`. |
| `shaderx` includes; `asset/importers/shader` | Partial | VFS, цикл и глубина уже существовали. Добавлены file/byte budgets, hashes всех transitive/engine includes, source maps, cache salt и восстановление после создания ранее отсутствующего include. `TestExpandedIncludesAreBoundedAndHashed`, `TestCacheSaltSeesTransitiveEngineIncludes`, `TestMissingTransitiveIncludeCreationRemovalAndRepair`. |
| `shaderx.Translate`, `gen.go` и reference validation | Partial | Проверяемый GLSL330/ESSL300 профиль, запрет unsupported/conditional schema, валидация каждого permitted combination обоих stages. `TestPortableChecksUseTokens`, `TestGlslangAcceptsEveryVariant`, diagnostics с backend/stage/variant/raw log. |
| `render/shader/abi.go`: std140 layout | Done для pure layout; GPU Unverified | Корректный расчёт сохранён. Исправлены границы записи vec3, integer/bool packing; проверены zero padding и смешанные последовательности. `TestMaterialStd140MatchesDriverAndReadsEveryKind` сверяет активные поля с настоящим драйвером. |
| `gen.go`: instance lanes/accessors | Broken | Float transport терял int32; macro substitution мог менять lexical scope. Теперь typed uvec4 attributes, точные integer bits и generated effective variables. Native/Web tests проверяют int32 за пределами точности float32. |
| Variants и program keys | Partial | Исходные 8 keywords / 32 combinations сохранены. Добавлены schema/profile checks, canonical masks, device/revision/layout validation; программы создаются для используемых комбинаций. |
| `asset/shader.go`: `BuildShader`, AESH | Broken по dependency boundary; Partial по формату | Runtime model больше не импортирует shaderx. AESH2 хранит подготовленные sources, schema, deps/hashes, layout/ABI, source maps и уровень проверки; bounded decoder и fuzz. |
| `asset/material.go`, material importer / loader | Broken | Нормализованный source text заменён AEMT1. Exact int32, explicit/default, unknown semantic data, aliases и unresolved IDs сохраняются. Runtime читает binary artifact. `TestMaterialDocumentBinaryAndSemanticRoundTrip`, loader/export tests. |
| `render/material/cache.go`: compile/link/reload | Partial | Старое продвижение отдельного материала не обеспечивало общую транзакцию. Теперь candidate → GPU validation → migration → frame-end commit, cancellation/device generations и освобождение старых ресурсов. |
| `asset/loader/material.go`: source/texture reload | Partial | Стабильная shared identity, отложенная публикация schema/values и уведомлений, отмена старого успеха при новой ошибке, сохранение queued definition при обновлении текстуры. Проверены также смена shader и одновременный reload его target. |
| Sprite pipeline / painter order | Partial | Существующий visual order сохранён. Materials входят в compatibility, а Sprite/Mesh2D объединены по painter order. Native/Web transparent A/B/A и Sprite/Mesh/Sprite image tests. |
| `modules/render2d`: назначение и overrides | Missing | Зарегистрированы material references, Mesh2D и MaterialOverride. Общие schema/property/Script/validation registries; explicit values отдельного узла, unknown retention, exact typed API, generation-aware propref. |
| `render/texture/data.go`: premultiply / Color storage | Broken | sRGB bytes умножались на alpha вне linear RGB; `SetPixel` писал linear Color в sRGB texture без encode. Исправлены оба преобразования; raw data textures выделены явно. Независимые CPU image references проверены на GL33 и двух браузерах. |
| Atlas / Sprite local UV / trim | Partial | Добавлены bounded sampling, original-local UV, crop dimensions/offset, AERG2 с чтением AERG1, FlipX/Y. Rotation вне поддержанного baseline явно отклоняется. Optional trim stream не меняет обычный 52-byte instance stream. |
| Multi-view / PixelSnap | Partial | Scoped camera clear сохраняет соседний viewport. PixelSnap учитывает physical pixels, offset viewport, DPR и ориентацию window/offscreen. Whole-image проверки. |
| Builtin materials | Missing | Unlit, Additive, Flash, Dissolve, Outline, PixelSnap проходят тот же source/import/artifact/runtime путь. Lit не зарегистрирован. `TestBuiltinMaterialsArePreparedUserPipelineArtifacts`, generated-artifact check, GPU image tests. |
| Material Inspector / shader panel / preview / History | Missing | Asset document через общие mutations/history; отдельный scene override; настоящий GPU preview или явная причина отсутствия; existing code editor, includes/Problems, Save/Autosave/NoWatch, searchable palette и persistent creation Undo. Native authoring workflow и новые RU/EN, dark/light goldens. |
| Tween / Animation / SDK / Script | Missing | Существующий propref расширен динамическими material properties. Binding пересобирается при generation; Play/Edit и prefab отделены. `tests/materials` проверяет Tween, Animation, reload и exact int32. |
| CLI/MCP | Missing | Команды shader/material зарегистрированы в существующем command registry, сохраняют atomic transactions, optimistic source hash, dirty in-memory documents, JSON diagnostics. CLI не импортирует editor/history. |
| Web context restore / pending reload | Unverified | Реальные события loss/restore, device generation invalidation, воссоздание программ/buffers/textures/VAO/bindings, last-good и bounded resource counts. Поддержка KHR polling реализована; реальный путь с доступным расширением отдельно остаётся Unverified в данном окружении. |
| Strict export | Missing для текущего shader/material source | Проверяются current source/includes, target artifacts, variants, dependencies, dirty buffers, schema/domain/overrides сцен и prefabs. Immutable artifact snapshot с проверкой hash/size исключает подмену bytes во время export. |

## Ошибки, найденные именно интеграцией

GPU-прогоны обнаружили cgo pointer violation при передаче inline Mesh2D lanes, повторное использование timer query после context restore, очистку всей цели второй камерой, неверный Y-sign у PixelSnap и reserved GLSL identifier `packed`, который glslang принимал, а Mesa отклонял. Все эти случаи имеют отдельные regression tests.

Authoring workflow обнаружил отсутствие открытия `.aeshader/.glsl/.aemat` в существующем code editor, отсутствие import после Save при NoWatch, потерю creation history, отсутствие прокрутки длинного Material Inspector и показ выдуманной колонки `:0` в Problems. Исправления сделаны в существующих общих механизмах.

Cross-layer audit обнаружил ещё четыре класса ошибок: newer-invalid material не отменял старый pending shader switch; texture refresh заменял queued material definition старым live value; same-batch target shader мог оставить материал на старой schema; export не вызывал component validators сцен и не проверял неиспользуемые prefabs. Для них добавлены постоянные отрицательные тесты, а не только ручные воспроизведения. Реальный browser allocation gate дополнительно обнаружил создание speculative diagnostics для корректного `SpriteRenderer.tint.a`: resolver сначала пытался найти компонент `SpriteRenderer.tint`. Проверка существования полного component prefix теперь предшествует созданию ошибки; full-name precedence и diagnostics неправильных paths сохранены. Race tests проверяют нулевые allocations для успешных color/vector lanes.

## Изменения изображений

Исправление linear premultiply меняет antialiasing-покрытие глифов и полупрозрачных текстур. Отдельное исправление `Data.SetPixel` меняет procedural checker colors, ранее ошибочно декодировавшиеся повторно как sRGB. Эти изменения не связаны с сортировкой/batching.

Все затронутые пары GL references просмотрены: before / after / difference. Непрозрачные панели, геометрия, clipping и painter order не изменились. Отдельно приняты MaterialOverride в Add Component и material counters в Profiler. Межбэкендовые material image tests используют независимые формулы с фиксированным tolerance; у них нет режима автоматического обновления reference image.

## Незакрытое доказательство и границы

В доступных Chromium/Firefox отсутствует `KHR_parallel_shader_compile`. Проверены synchronous driver fallback, deferred completion contract unit tests и реальная потеря контекста при ещё не принятом candidate. Нельзя называть это проверкой реально параллельной компиляции на драйвере с данным extension.

Не проверялись физические GPU, Safari/WebKit, Windows/macOS desktop. Linux GL33 и WebGL2 двух браузеров проверяются настоящими API. Atlas rotation, внешний Outline, Lit/lighting, particles, PostFX и публикация версии 0.12.0 не заявляются реализованными.

## Отделение screenshot fixtures от незавершённого проекта

Первый общий CI обнаружил, что `shots-check` копировал изменяемый пример платформера: присланный незавершённый уровень менял framing, иерархию и изображения документации. Установленного внешнего Agent CLI в окружении также не было, поэтому синтетический Context popup подменялся сообщением об отсутствии приложения. Эти изменения не принимаются как новые эталоны Stage 12a.

Для снимков выделена отдельная восстановленная контрольная сцена `tools/shots/testdata/main.aescene`:14 прежних top-level nodes из исходного `scene_tree.json`,464строки и пять значений, проверенных по старым Inspector/scene PNG. Удаление522строчного пользовательского хвоста и пять замен полностью совпадают с присланным stat527+/5−. Это восстановленный screenshot fixture, не утверждение о побайтовом наличии исходного Git blob. Подробная provenance и hashes находятся рядом с fixture. Исходные scene/script/meta пользователя остаются неизменными; копия screenshot-проекта не содержит дополнительный platform script. Остальные проверки примеров по-прежнему выполняются на настоящем рабочем примере.

В существующий явный `AEGO_SHOTS` режим через `Config.AgentBackend` передаётся read-only documentation backend: он не требует установленного CLI и всегда отказывает в запуске Agent conversation. Обычный редактор и явно переданный backend сохраняют прежний путь. Материальный GPU preview этим не подменяется. Контрольные изображения всё так же рисует настоящий native renderer.

Каждый documentation capture получает отдельный project/home через `mktemp`, с проверкой отсутствия editor lock перед запуском и удалением временной копии после него. Это устраняет повторное использование старых fixture-файлов между съёмками. Production lock не отключался; captures с lock-modal отклонены. Точный механизм повторного появления прежних файлов в проверяющем окружении не установлен. Новый `make shots-check` сравнил все 48 изображений без изменения эталонов или допусков и завершился успешно.

Browser screenshot smoke отдельно обнаружил fractional canvas origin: drawing buffer624×351 был корректен, но CSS y54.59375 заставлял capture расширяться до352строк. Screenshot-only выравнивание origin сохраняет размеры и pixels renderer; прежние bench64 image references проходят без изменения. Try-motion image отделён от проверки движения: реальный input→position assertion сохраняется, а UI reference снимается после reset/pause в фиксированной позе.
