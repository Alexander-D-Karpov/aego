MODULE_CI += test-camera

.PHONY: test-camera bench-camera

test-camera:
	go test ./modules/camera/... ./tests/camera/...
	go test -run 'Rewind' ./tests/integration

bench-camera:
	$(BENCH_PIN) go test -run '^$$' -bench BrainStep -benchmem -count=$(BENCH_COUNT) ./tests/camera
