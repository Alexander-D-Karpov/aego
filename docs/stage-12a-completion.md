# Stage 12a — completion report

## Состояние поставки

Полная реализация находится в patch и полных изменённых файлах поставки. Финальный общий `make -k ci` завершился с фактически наблюдаемым exit code 0. Shader/material GPU и полный authoring workflow проверены на доступных Linux GL33 и WebGL2 Chromium/Firefox; конкретные недоступные драйверные и платформенные проверки перечислены ниже. Исходный аудит — `stage-12a1-audit.md`, методика и before/after — `stage-12a-performance.md`, пользовательские форматы и команды — `materials-shaders.md`.

Source baseline: `ad23798da5abce89e850ceb9b312c2834daadc95`; checkpoint 12a1 в присланной истории — `132ddef`. Незавершённые пользовательские platformer scene/script/meta сохранены побайтно и исключены из Stage 12a patch. Изменения Stage 12a не коммитились и не публиковались; push/release/site deploy не выполнялись. Синтетический локальный baseline commit использован только для формирования и проверки patch, как указано в исходном аудите.

## Контракты реализации

- Authoring GLSL330 + Aego directives; подготовленный ESSL300 для WebGL2. Bounded tokenizer/parser, owner-relative includes, transitive hashes/maps, glslang pin 15.1.0 и все allowed combinations.
- Shader importer 3, generator 4, ABI 1, AESH2; material importer 2, runtime artifact AEMT1. Texture importer 3, atlas group 2, AERG2 (read AERG1). Runtime читает подготовленные артефакты без shaderx/importer/reference frontend.
- Declared schema, отдельный std140 packer, exact int32/bool typed lanes, maximum 8 authoring override fields при общем device attribute budget. Shader действительно читает effective values.
- Shader default → explicit material value → node override; Edit/Play/preview отдельны. Unknown semantic data и unresolved refs сохраняются; explicit equal-default остаётся explicit.
- Painter order layer/order/z/submission сохранён. Sprite/Mesh2D объединены по визуальному порядку; batch compatibility учитывает program generation, resources и state.
- Linear color / premultiplied contract, typed data textures, bounded atlas local UV, trim/flip, transparent offscreen references, физическая PixelSnap grid.
- Atomic candidate/schema/material publication на frame boundary, cancellation/device generations, target shader dependencies, last-good и resource retirement.
- Shared editor history/document/mutations, Inspector/GLSL Problems/real preview, Create и Add Component palette, Tween/Animation/Script/SDK, CLI/MCP и strict export.

## Как читать матрицу

48 строк соответствуют отдельным ID полного ТЗ. `Done` означает указанное выполненное доказательство, не обещание всех платформ и драйверов. `Unverified` относится к недоступному конкретному пути. Количество test functions/checks не отождествляется с количеством acceptance IDs. Headless/reference validation не называется GPU link; source goldens не заменяют image tests.

## Уровень I — pure/headless

| ID | Статус | Доказательство |
|---|---|---|
| A01 | Done | Declared material schema: tokenizer/parser учитывает comments, scopes, multiline declarations, все scalar/vector/color/sampler types. `TestCommentsDoNotDeclareMetadataOrEntries`, `TestTypedInstanceParametersAndLexicalScope`, generated sources. |
| A02 | Done | `TestDiagnostics`, `TestSchemaCannotDependOnPreprocessorOrLocalScope`, `TestPortableChecksUseTokens`: reserved/duplicate names, wrong defaults, nonfinite values, conditional schema отклоняются. |
| A03 | Done | `TestIncludes`, `TestExpandedIncludesAreBoundedAndHashed`, importer relative include tests: cycles/missing/depth/bytes/files/traversal; explicit ./../ от owner без выхода из namespace. |
| A04 | Done | `TestCacheSaltSeesTransitiveEngineIncludes`, `TestRelativeNestedIncludesPreserveTargetsMapsAndCache`, `TestMissingTransitiveIncludeCreationRemovalAndRepair`, end-to-end loader include swap. |
| A05 | Done | `TestMapLogPointsIntoInclude`, `TestReferenceDiagnosticsKeepTargetStageIncludeAndRawLog`, Problems `TestWhereOmitsUnknownSourcePositions`: исходный include/file/line, raw log, неизвестная column отсутствует. |
| A06 | Done | `TestGeneratedSourcesGolden`, `TestCommittedESShadersAreCurrent`, `TestGlslangAcceptsEveryVariant`, portable token tests; GLSL330/ESSL300 source generation deterministic. |
| A07 | Done | 8 keywords / 32 allowed combinations; canonical masks, unknown/forbidden variants rejected; `TestReloadVariantsMatchKeywordsNotBitPositions`, shaderx/material/export tests. |
| A08 | Done | `TestStd140RequiredMixedSequences`, `TestStd140PackingZeroPaddingAndIntegerWidths`, vec3-following-scalar regression; real GL driver reflection/readback. |
| A09 | Done | `TestMaterialDocumentBinaryAndSemanticRoundTrip`, `TestMaterialTypeErrorsAndUnresolvedReference`, retained-value tests: exact int32, explicit equal-default, unknown semantics, unresolved IDs. |
| A10 | Done | AESH2 bounded decoder: wrong version/layout/checksum/bounds/truncation; `TestShaderArtifactRejectsInvalidLayoutsAndMetadata`, `TestArtifactRoundTrip`, decoder fuzz. |
| A11 | Done | Migration same/new/removed/Former/default/type; typed explicit veto; loader pending-source cancellation, pending texture refresh, same-Swap shader target and cache dependency barrier regressions. |
| A12 | Done | Shared material document mutation/history; atomic/dry-run/optimistic revision; `TestMaterialCommandsUseDirtyDocumentAndRejectItsStaleRevision`; source disk remains unchanged in dirty editor host. |
| A13 | Done | Generation-aware propref, Tween/Animation integration, exact int32 animation and warmed zero-allocation binding; `tests/materials`, render2d propref test. |
| A14 | Done; finite campaigns | Source/include fuzz 15s / 103987 executions; material source 15s / 320553; AEMT 10s / 510221; AESH 10s / 9. Ни один finite fuzz run не доказывает отсутствие всех возможных ошибок. |

## Уровень II — GL33/WebGL2

| ID | Статус | Доказательство |
|---|---|---|
| G01 | Done | Custom fragment со штатным vertex и custom vertex/fragment компилируются в GL33/WebGL2. `TestMaterialCustomVertexPublishesAllEditedVaryings` проверяет actual readback UV/local/color/region/local-rect; driver debug regression без shader cache не даёт uninitialized warnings. |
| G02 | Done | `TestMaterialStd140MatchesDriverAndReadsEveryKind`, typed int32/bool image checks GL33 и обоих WebGL2 browsers. |
| G03 | Done | Driver uniform-block reflection сверяет offset/type/block size; declared schema и optimized-out params сохраняются независимо от GPU activity. |
| G04 | Done | Real reload rebinds blocks/samplers и заменяет locations; context restore пересоздаёт bindings. `TestRealReloadKeepsLastGoodRebindsAndReleasesGenerations`. |
| G05 | Done | Independent CPU linear-premultiplied image references: texture/tint/opacity, Flash/Dissolve alpha; прозрачная offscreen цель и checkerboard. |
| G06 | Done | Color upload linear-premultiply → sRGB encode; data textures linear/unpremult/noatlas; raw data vs color image cases и importer tests. |
| G07 | Done | Transparent A/B/A и Sprite/Mesh/Sprite whole-image tests. Material ID не является visual-order ключом. |
| G08 | Done | `TestIntegerOverridesAreTypedAndDoNotSplitBatch`, `TestLaneValuesDoNotBreakBatches`, paired 3000-sprite benchmark: одна совместимая группа остаётся одним batch. |
| G09 | Done | Compatibility includes textures/samplers/variant/blend/clip/target/layout; sprite state/batch tests и GPU ordered-material scenes. |
| G10 | Done | Authoring/schema/capability budget checks fail explicitly; typed uvec4 lanes use общий attribute budget, optional trim attribute проверяется вместе с основным layout. |
| G11 | Done; rotation rejected | Native/Web images для full/trim/local UV/FlipX/FlipY; bounded atlas sampling и interior Outline. Atlas rotation вне baseline явно отклоняется, не изображается поддержанным. |
| G12 | Done | `TestPixelSnapPhysicalGridAcrossDPRViewportAndTargets`, browser DPR2 offset viewport, multi-camera scoped clear и offscreen orientation. |
| G13 | Done | `scene/frame.TestTwoWorldsKeepIndependentMaterialTimeAndInstanceState`: A→B→paused B→A, разные Game/Real clocks и uploaded frame/lane bytes. Native pixel readback проверяет независимые preview clocks/overrides; Web fixture повторяет контракт. |
| G14 | Done | Broken driver reload сохраняет program/schema/UBO/explicit values; malformed/newer-invalid source отменяет старый success candidate. |
| G15 | Done | Generation/cancellation и stale deferred result tests; grouped target shader dependency tests. Реальная async extension ветка отдельно ограничена окружением. |
| G16 | Done | 16 native reload cycles и 12 browser cycles сохраняют GPU shader/pipeline/buffer counts; preview release cancels pending jobs. |
| G17 | Done | Оба браузера: реальные webglcontextlost/webglcontextrestored events, resources/frames/animation после restore; fixture и standalone export. |
| G18 | Done for available fallback; extension path Unverified | Real loss при submitted-but-uncommitted candidate; старое device generation не публикуется. KHR_parallel_shader_compile отсутствует в обоих браузерах; его реальная driver ветка не выполнялась. |
| G19 | Done | Sprite/Mesh2D реальные GPU draws, detached typed Mesh2D lane storage; scene и unused-prefab incompatible domain/override diagnostics блокируют export. |
| G20 | Done | Unlit/Additive/Flash/Dissolve/Outline/PixelSnap импортируются общими importer/artifact/runtime API. Lit не зарегистрирован; renderer не содержит builtin-name routing. |

## Уровень III — workflow/export

| ID | Статус | Доказательство |
|---|---|---|
| E01 | Done | `TestMaterialAndShaderCreationAndOverrideUseKeyboard` выполняет настоящий input cycle Create Shader → Create Material → add/assign, включая focused code editor; native/headless authoring workflow проверяет stable IDs, save/open и creation Undo/Redo. |
| E02 | Done | `TestMaterialTextureDropAndVariantEachHaveOneUndo`: настоящий mouse-down/move/up от Assets tile к texture picker, затем variant click; каждому действию соответствует отдельный Undo. Workflow дополнительно проверяет slider coalescing. |
| E03 | Done | Две ноды используют один material; override A не меняет B или shared asset. Typed render2d test и native editor workflow. |
| E04 | Done | `TestMaterialPlayPreservesExplicitPrefabOverridesAndDirtyState`: Tween и Animation в Play, explicit prefab overrides, scene/prefab bytes и dirty-state не меняются. `tests/materials` дополнительно проверяет reload binding и exact int32. |
| E05 | Done | Broken shader → last-good → fix, include Problems navigation; `TestShaderProblemOpensRealIncludeLine`, native workflow и code-error images. |
| E06 | Done | Preview использует real material renderer в собственной session; no-GPU placeholder с причиной, проверенный отдельным image и pure test. |
| E07 | Done | `TestMaterialCLIAndMCPUseOpenDocumentAndSceneHistory`: CLI Main→HTTP proxy и MCP Relay→material set/assign/validate; dirty open document и scene History, отдельный Undo. Dirty-host/atomic/optimistic/type/JSON negative tests дополняют transport. |
| E08 | Done | Final Linux/amd64 release export issues=[]; запуск из пустого cwd с PATH=/nonexistent и MESA_SHADER_CACHE_DISABLE=true:180 реальных GL33 кадров, exit 0, 0 errors/warnings. Trace: 180 phase.FrameBegin, 360 sprite views. Binary SHA256: 337f6ac40e84677bea87eb34f2c120d8f24cd1afa32de168906d8006835ccb81. |
| E09 | Done | Final static export в Chromium и Firefox: по 5 checks, Flash/Dissolve pixel changes до/после real loss/restore, frames continue, 0 console/network errors. Wasm SHA256: e8999b32823f97b0d32eec48945b4ccf623f9693c11f732d3999013f0a259d8f (27005288 bytes, gzip 6443393). |
| E10 | Done | Strict current source/dirty include/ESSL/variant/material/texture/dependency validation; last-good не обходит gate. Frozen snapshot hash/size, current .meta и все atlas members проверены. |
| E11 | Done | Отдельно собраны runtime/playground/try, gzip budgets 8/8/10 MiB сохранены. Runtime dependency denylist исключает importer/editor/native stack. |
| E12 | Done | MaterialOverride-specific Add Component и Shader/Material Create проходят клавиатурой, поиск получает focus, включая вызов из focused code editor. `TestMaterialAndShaderCreationAndOverrideUseKeyboard`. |
| E13 | Done | 16 real-GPU Inspector/draft/code-error/no-GPU images: EN/RU × light/dark. Reference images просмотрены; повторный тест без update PASS. |
| E14 | Done | Финальный `make -k ci`: observed exit 0, 1510.609 s. Штатные fuzz, depcheck, examples, native GL, 48 documentation images, browser smoke, profile size/allocation gates и 54 Chromium material checks выполнены; полный log/status и gate summary сохранены. |

## Недоступные и не заявленные проверки

`KHR_parallel_shader_compile` отсутствует в Chromium 141 и Firefox 142 этого окружения. Completion polling/cancellation contract проверен deferred backend tests; реальные sync fallback и context loss/restore проверены обоими браузерами. Ветка реально параллельной driver compilation остаётся Unverified.

Физические GPU, Safari/WebKit, Windows/macOS standalone здесь не проверены. Linux GL33/Mesa и два WebGL2 браузера реально создавали programs, рисовали и читали pixels. Для трёх browser-профилей выполнена отдельная последовательная before/after пара с одинаковыми условиями; один timing run на профиль не доказывает статистическое отсутствие регрессий. CPU/null и browser результаты имеют явно обозначенную область применимости; увеличение WebGL calls и Try callback p95 отражено в performance report. Atlas rotation и внешний Outline не входят в поддержанный v1 профиль; Lit/lighting, particles, PostFX и релиз всей 0.12.0 остаются за рамками этапа.

Ранний Chromium context restore timeout сохранён в evidence. Позднее focused прогон выявил точный protocol error: test harness мог вызвать restoreContext до окончания dispatch события contextlost. В harness добавлена явная следующая browser task после события; timeout и assertions не ослаблялись. Подтверждающие финальные browser результаты приведены отдельно, ранние неуспешные JSON сохранены. Ранняя попытка CI в ограниченном sandbox не могла открыть локальные sockets; полные прогоны используют разрешённое локальное окружение и хранят отдельные logs.

## Зафиксированные проверки и устранённые ошибки общего gate

Dedicated material GPU suite: **23 native top-level tests**, 0 failures, 0 skips; **54 checks в Chromium и 54 в Firefox**, все выполнены. Это числа тестов, а не число acceptance IDs. Дополнительный exact workflow suite выполняет четыре настоящих authoring-сценария: keyboard creation, Assets texture drag/drop + variant Undo, Tween/Animation Play с сохранением prefab overrides, CLI→HTTP и MCP→общая History. Последний targeted race-проход этих четырёх тестов завершился успешно за 21.433 s. Editor preview и native end-to-end workflow отдельно используют настоящий GL33.

Первый полный `make -k ci` завершился с exit 2 и пятью неуспешными targets. Они сохранены в исходном логе, а исправления подтверждены последующим полным CI:

| Target | Найденная причина | Исправление |
|---|---|---|
| `test-editor` | Try scene ссылалась на `demo.MaterialPulse`, а экспортируемое script type имеет полное имя `game.demo.MaterialPulse`. | Исправлена ссылка; тест проверяет реальный compiled type и изменение Flash более чем на 0.8 в обоих Try entry: `render2d/materials` и `engine.MaterialOverride`. RU counterpart отдельно прошёл strict docs check. |
| `golden` | Дополнительные состояния старых UI golden-тестов раскрылись после первого прохода; исправленный color/alpha contract меняет края glyphs. | Изображения просмотрены вручную. Восемь вторичных состояний приняты отдельно, повторный тест без update прошёл. |
| `shots-check` | Документационная сцена зависела от пользовательских несвязанных изменений platformer и доступности внешнего Agent CLI. | Изолированная, явно реконструированная fixture с проверкой на baseline; read-only documentation Agent не отправляет запросы. После повторного stale-lock modal каждый запуск получает новый private project/home через mktemp и проверяет отсутствие lock перед запуском; обычная защита редактора не изменена. |
| `web-smoke` | Дробный CSS origin давал лишнюю строку PNG; Try reference зависел от текущего physics tick. | Выравнивается только capture origin, прежние Playground PNG сохранены. Try сначала доказывает keyboard movement, затем reset/pause в одной JS task и проверяет frame 0; одно изображение с новым Material row просмотрено и принято отдельно. |
| `site-check` | Guide требовал регистрации material propref, а новая страница материалов нуждалась в полном RU counterpart. | Исправлены guide validation и локализованная страница, strict documentation check и site build завершились без ошибок. |

Для documentation fixture зафиксировано происхождение: она реконструирована из старых JSON/PNG и контекстного diff 527+/5−, а не объявлена утраченным original Git blob. На исходном baseline все 48 снимков проходят прежний порог; максимальное отличие 0.25%, проверенные области геометрии совпадают полностью. Все 48 финальных снимков просмотрены попарно. Обновлено 47, прежний `mixer-light.png` сохранён при разнице только live-counter. Итоговый shotdiff проходит без повышения прежнего порога 3%/channel24. Первые 24 light captures с предупреждением stale lock отклонены, сохранены как rejected и не использованы в качестве reference; точная причина появления старого lock не установлена. Однократная отдельная съёмка прошла, но полный CI воспроизвёл проблему при повторном использовании того же light path. Финальная fixture выдаёт новый mktemp path на каждый вызов и проверяет отсутствие lock до открытия. Точный механизм повторного появления прежних файлов в окружении не установлен; в коде нет восстановления lock-modal или embedded старого root. `make shots-check` с новой fixture прошёл за 215.475 s, все 48 изображений сравнены с прежними эталонами без обновления.

После полной сборки runtime allocation gate выявил 20 лишних allocations при разрешении корректного `SpriteRenderer.tint.a`. Убрано создание промежуточной ошибки для отсутствующего полного component prefix; resolution precedence и ошибки неверных путей сохранены. Тот же gate и allowlist дают 0 неразрешённых allocations. Общий runtime profile содержит 23 разрешённых first-use/high-water allocations за 600 кадров; собственный прогретый material/render путь измерен отдельно и даёт 0.

Две integration race-попытки остановлены как superseded: сначала из-за последующего изменения исходников, затем из-за несовпадения GOFLAGS. Они не считаются PASS или test failure. Финальный prime использует точно то же окружение, что CI: Go 1.27.0, `GOFLAGS=-buildvcs=false`, `AEGO_REQUIRE_GLSLANG=1`. Финальный отдельный integration race-прогон завершился успешно: `ok aego/tests/integration 878.021s`, exit 0. Его результат и результат полного CI хранятся отдельно; итоговый общий CI повторно выполнил integration-пакет, так как кэш этого пакета не использован.

### Повторный общий прогон и последние исправления

Прогон `aego-make-ci-clean.log` от 17:36–18:02 UTC выполнил integration race за 837.689 s и второй integration profile за 78.944 s. Fuzz, native goldens, GL33 workflow, весь browser smoke/audio/theme/benchmark и 54 material checks прошли. Остались два неуспешных target:

- `shots-check`: на light capture повторно появился stale-lock modal. Тематических фиксированных путей оказалось недостаточно; конечная Make fixture создаёт уникальный каталог на каждый вызов. Сам механизм повторного появления старых файлов в окружении не объявляется доказанной ошибкой production lock кода.
- `site-check`: HTML-функции ссылались на несуществующий `#example` у material module. Генератор теперь создаёт стабильные anchors у runnable/checked AES snippets и направляет ссылку на первый из них. EN/RU regression test и полный `make site-check` прошли. В nav golden приняты после просмотра ровно четыре добавления: Materials guide, `aego/material`, MaterialOverride и Mesh2D, без удалений/перестановок.

Лог второго общего прогона заканчивается сообщением make об ошибках. Его оригинальный OS wait status был утрачен вместе с monitoring session; recovery JSON честно содержит `exit_code:null`, `inferred_exit_code:2`, а не выдуманный наблюдаемый status. Финальный общий прогон после двух исправлений записывает фактический результат `process.wait` в отдельные `aego-make-ci-verified` log/status.

### Повторный CI и audio harness

Прогон `aego-make-ci-verified` завершился с наблюдаемым exit code 2 за 1548.476 s. Race integration (878.754 s), non-native integration (83.690 s), native goldens, все 48 documentation screenshots, strict site check, export и 54 Chromium material checks прошли. Единственный failed target — `web-smoke`: его audio demo накопил 31 underrun. Messages/SAB transport дали 0. Следующие внутри этого target theme и benchmark команды не выполнялись; подробный partial-gate summary сохранён отдельно как `aego-ci-gate-summary-audio-failed.json`.

Проверка исходников обнаружила ошибку самого harness: demo читал состояние и проверял autoplay через Playwright `evaluate`/`waitForFunction`, предоставлявшие синтетическую пользовательскую активацию; transport уже использовал CDP с `userGesture:false`. Теперь обе ветки используют один CDP helper, а demo явно проверяет отсутствие `navigator.userActivation.hasBeenActive` до настоящего клика. Boot/state/mute/autoplay probes не активируют страницу. Cumulative underrun counter, требование 0, глубины очередей, длительности, RMS/mute/reload checks не изменены. Чтение готовности выполняется polling с интервалом 100 ms вместо прежнего Playwright RAF polling.

Отдельный полный исправленный audio gate прошёл за 28.708 s: messages/shared/demo — 0 underrun; перед кликом `page activated false`. Диагностический запуск прежнего demo также прошёл с 0 и сохранил startup RAF/long-task trace. Эти данные не устанавливают точную причину прежних 31 underrun: это остаётся неуспешным конкретным прогоном, а не автоматически объявляется доказанным следствием synthetic gesture. Старые failure logs сохранены. Production audio/pump/worklet код не изменён. Общий CI с исправленным harness записывается отдельно в `aego-make-ci-accepted`.

### Итоговый полный CI

Последний прогон начат **2026-10-30T18:45:03.419083+00:00**, завершён **2026-10-30T19:10:14.027968+00:00**. Наблюдаемый результат `process.wait`: **exit code 0**, wall time **1510.609 s**. Это отдельный полный `make -k ci` после исправлений, не вывод из предыдущих targeted checks или восстановленного статуса неуспешной попытки.

Результаты integration-профилей в итоговом логе: race **852.030s**, non-native **83.762s**. Численная длительность означает фактическое исполнение; `(cached)` означает использование Go test cache и не объявляется повторным выполнением. В предыдущем сохранённом прогоне оба профиля реально выполнились за 878.754 s и 83.690 s. Для остальных Go packages использование cache также явно видно в полном логе. Проверены штатные targets из Makefile, включая pinned reference validator, generated materials, build/vet/lint/depcheck/i18n, editor/runtime/wasm profiles, strict allocation smoke, fuzz, native goldens, все 48 documentation screenshots, GL33 workflow, browser smoke, site checks, examples/export, module checks и material WebGL fixture. Fuzz duration, image tolerances и wasm budgets не повышались.

Native graphics выполнялись через SDL offscreen/EGL и Mesa llvmpipe; browser gate использовал Chromium 141. Отдельные Firefox и standalone export результаты приведены в E08/E09 и GPU evidence. Отсутствие `KHR_parallel_shader_compile`, физических GPU или других платформ не превращается в успешную проверку этих путей. Условные browser skips/ограничения явно сохранены в gate summary.

Итоговые доказательства: `evidence/aego-make-ci-accepted.log`, `evidence/aego-make-ci-accepted-status.json`, `evidence/aego-ci-gate-summary.json`. Предыдущие неуспешные и superseded попытки сохранены под другими именами.

При сборке локального сайта gendocs сообщил `no download buttons: invalid character '<' looking for beginning of value`: внешние release metadata не удалось разобрать как JSON, поэтому кнопки скачивания готовых релизов не построены. Это сохранённое нефатальное ограничение внешних данных; materials workflow, локальная документация, export и CI завершились успешно. Деплой сайта и проверка внешнего release endpoint не выполнялись.

## Воспроизводимые команды

```sh
make shader-validator
make materials-check
make ci

# Реальный GL33 workflow; не заменяется headless режимом.
go test -tags aego_gltest ./app/edit -run '^TestMaterialAuthoringWorkflowNative$' -count=1 -v

# Полный новый пример через обычные инструменты.
go run ./cmd/aego edit examples/materials
go run ./cmd/aego export examples/materials --target linux/amd64 --release --out /tmp/materials-native
go run ./cmd/aego export examples/materials --target web --release --out /tmp/materials-web
```

Для headless Linux с Mesa следует выбрать доступный EGL backend, например SDL `offscreen`, а не считать отсутствие GLX успешным GPU-тестом. Export workflow runner `tools/websmoke/export-materials.mjs` обслуживает только готовый export directory; ему не нужен importer endpoint.

## Содержимое поставки

`stage12a.patch` — полный binary-capable diff относительно присланного снимка. `changed-files/` содержит полные конечные файлы; `manifest.json` — пути и SHA256. В `evidence/` находятся логи проверок, экспортные отчёты, before/after JSON, driver/browser результаты. Исходники всех тестов, shader artifacts, fixture/golden images и демонстрации включены в patch. Логи не подменяют исходники и assertions.
