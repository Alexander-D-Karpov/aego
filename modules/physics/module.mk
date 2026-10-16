BENCH_PHYSICS_PKGS += ./modules/physics/world ./demos/physics2d
MODULE_CI += demo-physics example-sandbox check-physics
MODULE_BENCH_ALLOCS += bench-allocs-physics
MODULE_BENCH_BASELINE += bench-baseline-physics
MODULE_BENCH_COMPARE += ./tests/integration:EditorFrame

.PHONY: bench-physics-run bench-physics bench-allocs-physics bench-baseline-physics demo-physics example-sandbox check-physics

bench-physics-run:
	mkdir -p $(BENCH_DIR)
	$(BENCH_PIN) go test -run '^$$' -bench . -benchmem -count=$(BENCH_COUNT) $(BENCH_PHYSICS_PKGS) > $(BENCH_OUT)
	$(BENCH_PIN) go test -run '^$$' -bench Platformer -benchmem -count=$(BENCH_COUNT) ./tests/integration >> $(BENCH_OUT)

bench-physics:
	$(MAKE) bench-physics-run BENCH_OUT=$(BENCH_DIR)/physics.txt
	$(BENCH_GATE) -base $(BENCH_DIR)/physics.base.txt -head $(BENCH_DIR)/physics.txt

bench-baseline-physics:
	$(MAKE) bench-physics-run BENCH_OUT=$(BENCH_DIR)/physics.base.txt

bench-allocs-physics:
	$(MAKE) bench-physics-run BENCH_OUT=$(BENCH_DIR)/physics.allocs.txt BENCH_COUNT=1
	$(BENCH_GATE) -time=false -base $(BENCH_DIR)/physics.base.txt -head $(BENCH_DIR)/physics.allocs.txt

demo-physics: bootstrap
	go test -run TestSceneRunsHeadless ./demos/physics2d
	go run ./cmd/sandbox --demo physics2d-headless --frames 600

example-sandbox: cli
	.cache/bin/aego check examples/physics-sandbox
	.cache/bin/aego run examples/physics-sandbox --headless --frames 600

check-physics: cli
	.cache/bin/aego test examples/physics-sandbox
