.PHONY: test harness diff bench bench-vs-c bench-vs-c-enc bench-vs-c-dec vet fmt

# Reference harnesses around the stock Apache-2.0 opencore-amrnb shared library
# (encoder and decoder), built from testdata/refharness by `make harness`. Only
# the runtime library is needed (Debian/Ubuntu: libopencore-amrnb0). On other
# systems override the link flags, e.g. on macOS with Homebrew:
#   make harness AMRNB_LIBS="-L/opt/homebrew/lib -lopencore-amrnb"
HARNESS_DIR := testdata/refharness
AMRNB_LIBS ?= -l:libopencore-amrnb.so.0
AMRNB_DIFF ?= $(HARNESS_DIR)/bin/amrnbdec
AMRNB_ENC ?= $(HARNESS_DIR)/bin/amrnbenc
AMRNB_BENCH_FRAMES ?= 4000

# Unit tests (pure Go, no external dependencies).
test:
	go test .

# Build the reference harnesses.
harness: $(HARNESS_DIR)/bin/amrnbdec $(HARNESS_DIR)/bin/amrnbenc

$(HARNESS_DIR)/bin/amrnbdec: $(HARNESS_DIR)/amrnbdec.c
	@mkdir -p $(@D)
	$(CC) $(CFLAGS) -O2 -o $@ $< $(AMRNB_LIBS)

$(HARNESS_DIR)/bin/amrnbenc: $(HARNESS_DIR)/amrnbenc.c
	@mkdir -p $(@D)
	$(CC) $(CFLAGS) -O2 -o $@ $< $(AMRNB_LIBS)

# Bit-exact differential tests against the reference harnesses.
diff: harness
	AMRNB_DIFF=$(abspath $(AMRNB_DIFF)) AMRNB_ENC=$(abspath $(AMRNB_ENC)) \
		go test -count=1 -run 'DiffAgainstCReference' -v .

# Go micro-benchmarks: encode/decode ns/op + xRT real-time factor, per mode.
bench:
	go test -bench 'BenchmarkEncode|BenchmarkDecode' -benchmem .

# Go-vs-C speed comparison (both directions). Each sub-test SKIPs unless its
# harness env var is set, so this prints whichever harnesses are available.
bench-vs-c: harness bench-vs-c-enc bench-vs-c-dec

# Encoder Go-vs-opencore-amrnb per-mode table.
bench-vs-c-enc:
	AMRNB_ENC=$(abspath $(AMRNB_ENC)) AMRNB_BENCH_FRAMES=$(AMRNB_BENCH_FRAMES) \
		go test -count=1 -run TestEncSpeedVsCReference -v .

# Decoder Go-vs-opencore-amrnb per-mode table.
bench-vs-c-dec:
	AMRNB_DIFF=$(abspath $(AMRNB_DIFF)) AMRNB_BENCH_FRAMES=$(AMRNB_BENCH_FRAMES) \
		go test -count=1 -run TestDecSpeedVsCReference -v .

vet:
	go vet ./...

fmt:
	gofmt -l .
