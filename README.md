# goamr-nb

A **pure-Go AMR-NB (3GPP TS 26.071 / 26.090) codec** — no cgo. Implements all eight
speech modes (4.75–12.2 kbit/s) plus RFC 4867 RTP payload framing in both
octet-aligned and bandwidth-efficient formats, bit-exact with `opencore-amrnb`.

Every Go AMR-NB option on pkg.go.dev is a cgo wrapper around the C `opencore-amrnb`
library. This package is a hand-port of that fixed-point reference to Go, so it
builds with `CGO_ENABLED=0` and cross-compiles anywhere Go does. It mirrors the
sibling [`goamr-wb`](../goamr-wb) wideband codec in structure, API, and testing.

> **Status:** complete and bit-exact. All eight speech modes encode and decode,
> validated byte-for-byte against `opencore-amrnb`: the **encoder is bit-identical
> for all 8 modes**, and the **decoder produces bit-exact synthesis**. The full
> fixed-point DSP — LP analysis, LSF split-VQ quantization, open/closed-loop pitch,
> the five per-mode algebraic codebooks, gain quantization, synthesis, and
> post-filter — is ported function-by-function from the reference, with RFC 4867
> framing (both formats), the ETSI fixed-point operators, and AVX2-accelerated hot
> kernels. (DTX/VAD is always-active; comfort-noise SID is not yet emitted.)

## Install

```bash
go get github.com/VoiceBlender/goamr-nb
```

## Usage

```go
import amrnb "github.com/VoiceBlender/goamr-nb"

enc, _ := amrnb.NewEncoder(amrnb.EncoderConfig{Mode: amrnb.Mode1220, OctetAligned: true})
dec := amrnb.NewDecoder(amrnb.DecoderConfig{OctetAligned: true})

// One 20 ms frame = 160 samples of 8 kHz mono PCM (int16).
payload, _ := enc.Encode(frame)   // RFC 4867 RTP payload (CMR + ToC + speech bits)
pcm, _ := dec.Decode(payload)     // back to 160 PCM samples
```

`Encode` takes exactly `amrnb.FrameSamples` (160) samples; `Decode` accepts a full
RFC 4867 payload and returns concatenated PCM for every frame it carries.

| Mode | kbit/s | bits/frame | bytes/frame |
|---|---|---|---|
| `Mode0475` | 4.75 | 95 | 12 |
| `Mode0515` | 5.15 | 103 | 13 |
| `Mode0590` | 5.90 | 118 | 15 |
| `Mode0670` | 6.70 | 134 | 17 |
| `Mode0740` | 7.40 | 148 | 19 |
| `Mode0795` | 7.95 | 159 | 20 |
| `Mode1020` | 10.2 | 204 | 26 |
| `Mode1220` | 12.2 | 244 | 31 |

## Performance

Single-core, per-frame timings vs the Apache-2.0 `opencore-amrnb` C reference,
measured on an **AMD Ryzen 9 7900** (Zen 4, AVX2), Go 1.26.3, Linux/amd64. `×RT`
is the real-time factor — how many times faster than real time one core runs (a
20 ms frame's budget is 20 ms, so `×RT = 20e6 / ns-per-frame`). `Go/C` is the
per-frame time ratio (lower is better); the C side's per-frame cost is isolated
with a two-point-slope measurement to cancel process startup.

**Encode** — pure Go is within ~10–25 % of hand-optimized fixed-point C:

| Mode | Go ns/frame | Go ×RT | C ×RT | Go/C |
|---|---|---|---|---|
| MR475 | 43 300 | 462× | 528× | 1.14 |
| MR515 | 33 000 | 605× | 659× | 1.09 |
| MR59  | 38 500 | 519× | 560× | 1.08 |
| MR67  | 50 000 | 400× | 432× | 1.08 |
| MR74  | 48 100 | 416× | 456× | 1.10 |
| MR795 | 51 300 | 390× | 433× | 1.11 |
| MR102 | 51 100 | 391× | 465× | 1.19 |
| MR122 | 50 900 | 393× | 496× | 1.26 |

**Decode** — pure Go is *faster* than the C reference:

| Mode | Go ns/frame | Go ×RT | C ×RT | Go/C |
|---|---|---|---|---|
| MR475 | 6 790 | 2944× | 2296× | 0.78 |
| MR515 | 6 550 | 3052× | 2304× | 0.75 |
| MR59  | 6 560 | 3049× | 2259× | 0.74 |
| MR67  | 6 540 | 3061× | 2319× | 0.76 |
| MR74  | 6 670 | 3000× | 2428× | 0.81 |
| MR795 | 6 850 | 2922× | 2251× | 0.77 |
| MR102 | 6 900 | 2899× | 2370× | 0.82 |
| MR122 | 6 960 | 2874× | 2543× | 0.88 |

Decode is ~5–8× faster than encode for the same mode — inherent to ACELP: the
encoder *searches* for parameters by analysis-by-synthesis (open/closed-loop
pitch, the algebraic pulse search, split-VQ codebooks), while the decoder just
*applies* the chosen ones. The C reference shows the same asymmetry. The hot FIR
correlations (`autocorr`, open-loop pitch, `cor_h_x`) and the synthesis filter
run through AVX2 kernels (`firRaw`/`firDot`), fuzz-tested bit-identical to the
scalar fallback; on non-amd64 targets the portable path is used. Caveat: the C
timings cross a subprocess pipe per frame, a small per-frame I/O cost absent from
the in-process Go number, so the comparison modestly favors Go.

Reproduce with `make bench` (Go-only `×RT`) and `make bench-vs-c` (needs the C
harnesses; see below).

## Tests & benchmarks

```bash
# Unit tests (pure Go, no external dependencies)
go test .          # or: make test

# Go micro-benchmarks (encode/decode ns/op + xRT real-time factor, per mode)
go test -bench 'BenchmarkEncode|BenchmarkDecode' -benchmem .   # or: make bench
```

### Differential & speed-vs-C tests (optional, need the C reference)

The bit-exact differential tests and the Go-vs-C speed comparison run a locally
built harness around the Apache-2.0 `opencore-amrnb` C source (which provides both
encoder and decoder). They **skip** unless pointed at that harness:

```bash
# Encoder: all 8 modes bit-exact vs opencore-amrnb
AMRNB_ENC=/path/to/enc-harness go test -run TestEncDiffAgainstCReference .
# Decoder: bit-exact PCM vs opencore-amrnb
AMRNB_DIFF=/path/to/dec-harness go test -run TestDiffAgainstCReference .
# or, via the Makefile (defaults to harnesses in /tmp; override as shown):
make bench-vs-c AMRNB_ENC=/path/to/enc-harness AMRNB_DIFF=/path/to/dec-harness
```

The encoder harness reads 160 int16 LE per frame on stdin (mode as argv[1]) and
writes `[ToC byte][packed speech bytes]` per frame; the decoder harness reads
speech bytes per frame on stdin and writes 160 int16 LE per frame.
`AMRNB_BENCH_FRAMES` tunes the comparison's frame count (default 4000).

## License

Apache-2.0. This is a derivative of the Apache-2.0 `opencore-amrnb` reference —
see [NOTICE](NOTICE) for attribution and [LICENSE](LICENSE) for terms.
