MODULE_CI += test-animation
MODULE_BENCH_ALLOCS += bench-allocs-animation

.PHONY: test-animation bench-animation bench-allocs-animation

test-animation:
	go test ./anim/... ./asset/importers/animclip/... ./asset/importers/animctrl/... ./modules/animation/... ./tests/animation/...

bench-animation:
	$(BENCH_PIN) go test -run '^$$' -bench . -benchmem -count=$(BENCH_COUNT) ./anim ./tests/animation

bench-allocs-animation:
	mkdir -p $(BENCH_DIR)
	go test -run '^$$' -bench '^Benchmark(Players|Animator)100$$' -benchmem -count=1 ./tests/animation | tee $(BENCH_DIR)/animation.allocs.txt
	@awk '/^Benchmark(Players|Animator)100/ { seen++; for (i = 2; i <= NF; i++) if ($$i == "allocs/op" && $$(i-1) + 0 != 0) bad = 1 } END { if (seen != 2 || bad) { print "BenchmarkPlayers100 or BenchmarkAnimator100 allocates or did not run"; exit 1 } }' $(BENCH_DIR)/animation.allocs.txt
