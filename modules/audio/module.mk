MODULE_CI += demo-audio
MODULE_BENCH_ALLOCS += bench-allocs-audio

.PHONY: bench-audio bench-allocs-audio demo-audio

bench-audio:
	$(BENCH_PIN) go test -run '^$$' -bench . -benchmem -count=$(BENCH_COUNT) ./audio/mix

bench-allocs-audio:
	mkdir -p $(BENCH_DIR)
	go test -run '^$$' -bench '^BenchmarkMix64$$' -benchmem -count=1 ./audio/mix | tee $(BENCH_DIR)/audio.allocs.txt
	@awk '/^BenchmarkMix64/ { seen = 1; for (i = 2; i <= NF; i++) if ($$i == "allocs/op" && $$(i-1) + 0 != 0) bad = 1 } END { if (!seen || bad) { print "BenchmarkMix64 allocates or did not run"; exit 1 } }' $(BENCH_DIR)/audio.allocs.txt

demo-audio: bootstrap
	go run ./cmd/sandbox --demo audio --headless --frames 300
