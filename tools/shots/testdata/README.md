# Documentation screenshot scene

`main.aescene` is a reconstructed screenshot fixture for the existing
`app/edit/shots.go` scenarios. It is not an original Git blob. `make shots` and
`make shots-check` copy the platformer project into the theme's own temporary
directory, `.cache/shots/dark/project` or `.cache/shots/light/project`, overlay
this scene, and remove the unrelated `scripts/platform` directory from that
temporary copy. Each theme has separate editor locks, native build files and
saved UI state. The source project remains untouched.

The capture scene’s player has a tinted sprite. Its matching pre-Flash
`player_hurt.aeanim` is copied alongside the scene so the Animator can
resolve the Hurt track during Play. The playable example uses its own
per-character Flash material and clip.

The supplied Stage 12a worktree already contained unfinished platformer edits:
the scene had a longer level and an untracked platform script. Capturing that
worktree directly changed documentation images for reasons unrelated to
materials. The supplied archive did not include the earlier scene or Git objects;
its context recorded only the change summary: 527 added and five removed lines.

## Reconstruction evidence

The supplied scene has 986 lines. Its additional nodes, starting at `Ground3`
after `ArenaZone`, occupy 522 lines. Removing those nodes leaves a 464-line scene
with the same 14 top-level node IDs and order as the pre-existing
`tests/agent/testdata/scene_tree.json`. The pre-existing `scene_get_player.json`
also records Player's transform, components and script values. No platform script
or added platformer node is included in this fixture.

Five values in the remaining scene were restored using the pre-existing
documentation images. Together with the 522 removed lines, these five replacements
account for the supplied change summary exactly.

| Field | Fixture value | Evidence in the pre-existing image |
|---|---|---|
| `Camera / VirtualCamera2D.bounds` | `[-20, -14, 40, 19.5]` | `camera-follow.png`: all four values are visible in Inspector. |
| `ArenaZone / AudioZone2D.area` | `[-6, -3, 12, 7]` | `camera-zone.png`: all four values are visible in Inspector. |
| `ArenaZone / CameraZone2D.area` | `[-6, -3, 12, 7]` | `camera-zone.png`: its rectangle matches the audio zone and spans 12 grid units. |
| `Pit / KillZone2D.area` | `[-20, -1, 40, 2]` | `editor.png`: its rectangle spans 40 world units. |
| `GroundRight / EdgeCollider2D.points`, final point | `[6.5, -14]` | `editor.png`: the right wall reaches world Y = -10, the same height as the left wall; GroundRight has Y = 4. |

The last three values are reconstructed from the image geometry. The unavailable
original Git blob cannot be used to confirm byte-for-byte identity. Changes to
this fixture should be deliberate changes to the documentation scenario, not a
copy of the current editable example.

These SHA-256 digests identify the evidence before subsequent Stage 12a
documentation-image and CLI-fixture updates:

| Evidence | SHA-256 |
|---|---|
| `docs/shots/camera-follow.png` | `422826fab143aeb7542aa239d7c5b93f4f29779d8317622c10c03a7f7bd5feb0` |
| `docs/shots/camera-zone.png` | `09b95f2140c8bdc10a3814771a133051968c9013928bd66d3661057471bc0d15` |
| `docs/shots/editor.png` | `c4e57fab3f8bac01a87577c0bcabe899ded1079b80fefb34302f9121fe2f1cd9` |
| `tests/agent/testdata/scene_tree.json` | `6a10af03da1189391d8d086198da9e35099ec0cb544c279e63f53d41bebb4edc` |
| `tests/agent/testdata/scene_get_player.json` | `a091179a0e845f444a681c2241ed829f3e73188bf903ff10d21d3d9f1fe4debc` |

Initial fixture SHA-256:
`09b14a213fa5f4fb11a65b74b0b081bdb91995f7c88791c5e76f22f3432fc8ff`.

The supplied baseline editor captured all 48 dark/light screenshots with this
fixture and its unchanged capture scenarios. A temporary Go overlay supplied the
existing fake Agent backend only for `AEGO_SHOTS`; it changed no renderer or scene
code and required no external agent service. All 48 images passed the existing
`tools/shotdiff` comparison against the pre-existing documentation images:
channel tolerance 24, maximum changed-pixel allowance 3%, largest observed
difference 0.25%. No reference images or comparison limits were changed for that
validation.

The setup, frame counts, selections, Play operations and capture path remain the
real editor scenarios in `app/edit/shots.go`. Snapshot comparisons must still
inspect any visual changes before accepting new reference images.
