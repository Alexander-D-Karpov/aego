MODULE_CI += test-camera

.PHONY: test-camera bench-camera

test-camera:
	go test -count=1 ./modules/camera/... ./tests/camera/...
	go test -count=1 -run 'Rewind' ./tests/integration

bench-camera:
	$(BENCH_PIN) go test -run '^$$' -bench BrainStep -benchmem -count=$(BENCH_COUNT) ./tests/camera
