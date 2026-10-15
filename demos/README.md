# demos

Bare-engine programs run by `cmd/sandbox` (`make run DEMO=<name>`). Every directory with Go files must be registered in `demos.Registry` (`cmd/sandbox/registry_test.go`).

| Demo | What it shows | Run by |
|---|---|---|
| basic | window, vsync, quad shader from `demos/res/shaders` | `tests/integration/headless_test.go` |
| headless | `basic` on the null GPU with `--strict-allocs` | `make smoke` |
| input | keyboard, mouse and text input events | manual |
| resize | window resize and render target recreation | manual |
| sprites | texture atlas and sprite batch | manual |
| camera | 2D camera driven by the mouse | manual |
| layers | sorting layers and the debug font | manual |
| text | debug font rendering | manual |
| offscreen | render to texture | manual |
| stress | sprite batch at `MaxSprites` | `tools/bench`, `make bench` |
| stress-scene | scene with `modules/render2d` sprite renderers | `tools/bench -demo stress-scene` |
| scene-load | loading `res://scenes/test.aescene` through the pipeline | manual |
| asset-hot-reload | `asset/pipeline` hot reload | manual |
| hierarchy | node hierarchy and transforms | manual |
| mesh | 3D mesh | manual |
| mixed | 2D and 3D in one frame | manual |
| stress-mesh | many meshes | manual |
| physics2d | `modules/physics/world` without the module | `make demo-physics`, `make bench-physics` |

`demos/res` holds the shaders and the test scene used by the demos, `tools/bench` and the golden tests. `demos/internal` holds helpers shared by the demos only.
