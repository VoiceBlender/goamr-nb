.PHONY: test bench bench-vs-c bench-vs-c-enc bench-vs-c-dec vet fmt

# Paths to the locally built Apache-2.0 opencore-amrnb reference harnesses, and
# the comparison workload size. Override on the command line, e.g.
#   make bench-vs-c AMRNB_ENC=/path/to/enc AMRNB_DIFF=/path/to/dec
# or via the environment. The ?= defaults assume the harnesses live in /tmp.
AMRNB_ENC ?= /tmp/amrnbenc
AMRNB_DIFF ?= /tmp/amrnbdiff
AMRNB_BENCH_FRAMES ?= 4000

# Unit tests (pure Go, no external dependencies).
test:
	go test .

# Go micro-benchmarks: encode/decode ns/op + xRT real-time factor, per mode.
bench:
	go test -bench 'BenchmarkEncode|BenchmarkDecode' -benchmem .

# Go-vs-C speed comparison (both directions). Each sub-test SKIPs unless its
# harness env var is set, so this prints whichever harnesses are available.
bench-vs-c: bench-vs-c-enc bench-vs-c-dec

# Encoder Go-vs-opencore-amrnb per-mode table.
bench-vs-c-enc:
	AMRNB_ENC=$(AMRNB_ENC) AMRNB_BENCH_FRAMES=$(AMRNB_BENCH_FRAMES) \
		go test -run TestEncSpeedVsCReference -v .

# Decoder Go-vs-opencore-amrnb per-mode table.
bench-vs-c-dec:
	AMRNB_DIFF=$(AMRNB_DIFF) AMRNB_BENCH_FRAMES=$(AMRNB_BENCH_FRAMES) \
		go test -run TestDecSpeedVsCReference -v .

vet:
	go vet ./...

fmt:
	gofmt -l .
