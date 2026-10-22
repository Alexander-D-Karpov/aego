MODULE_CI += test-gameplay

.PHONY: test-gameplay bench-gameplay

test-gameplay:
	go test ./modules/gameplay/... ./tests/gameplay/...

bench-gameplay:
	$(BENCH_PIN) go test -run '^$$' -bench Motion100 -benchmem -count=$(BENCH_COUNT) ./tests/gameplay
