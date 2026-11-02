# goamr-nb

A **pure-Go AMR-NB (3GPP TS 26.071 / 26.090) codec** — no cgo. Encoder and decoder for
all eight speech modes (4.75–12.2 kbit/s) with VAD, DTX and comfort noise, plus RFC 4867
RTP payload framing (octet-aligned and bandwidth-efficient, several frames per packet,
frame CRCs, robust sorting, mode-set and mode-change rules) and the `.amr` file format.
Frame interleaving is not supported.

Every Go AMR-NB option on pkg.go.dev is a cgo wrapper around the C `opencore-amrnb`
library. This package is a hand-port of that fixed-point reference to Go, so it builds
with `CGO_ENABLED=0` and cross-compiles anywhere Go does. It mirrors the sibling
[`goamr-wb`](https://github.com/VoiceBlender/goamr-wb) wideband codec in structure, API
and testing.

The decoder is **bit-exact** with `opencore-amrnb`'s `Decoder_Interface_Decode` in all
eight modes for speech frames, lost frames (concealment), mode switches and comfort noise
(SID and NO_DATA frames), including its 13-bit output. Known differences:

- **Q bit:** frames with Q=0 are decoded as bad frames (RFC 4867). `Decoder_Interface_Decode`
  ignores the Q bit, so bad speech and SID frames have no reference to compare against.
- **SPEECH_LOST:** frame type 14 (3GPP TS 26.101) is decoded as a bad frame in the previous
  mode. `Decoder_Interface_Decode` does not decode frame types 9–14 at all.

The encoder is **bit-exact** with `opencore-amrnb`'s `Encoder_Interface_Encode`, with DTX
on or off, in all eight modes, from quiet to clipped input, on tones and DTMF, and across
mode switches, including the VAD decisions (VAD option 1), SID frames and NO_DATA frames.

Both claims are checked by the differential tests below. Like `opencore-amrnb`, this
package implements no encoder or decoder homing frames: they are coded like any other
input.

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

`Encode` takes one or more 20 ms frames, a multiple of `amrnb.FrameSamples` (160) samples,
and returns one payload carrying them all (e.g. 480 samples for `ptime:60`); `Decode`
accepts a full RFC 4867 payload and returns concatenated PCM for every frame it carries.
`EncodeTo` and `DecodeTo` do the same without allocating; size the payload buffer with
`amrnb.MaxPayloadBytesFor(frames)`. `Encoder.Mode` returns the mode of the next frame.
`Encoder.Reset` and `Decoder.Reset` clear the codec state: the encoder keeps its mode, mode
request and DTX setting, and the decoder forgets the peer's mode request.

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

### Payload format (SDP parameters)

The config fields mirror the RFC 4867 `fmtp` parameters, and leaving them unset matches
their absence. Both sides must match what the session negotiated:

| SDP parameter | `EncoderConfig` | `DecoderConfig` |
|---|---|---|
| `octet-align=1` | `OctetAligned` | `OctetAligned` |
| `crc=1` | `CRC` | `CRC`: a frame failing its CRC is decoded as a bad frame |
| `robust-sorting=1` | `RobustSorting` | `RobustSorting` |
| `mode-set=0,2,5` | `ModeSet: []amrnb.Mode{0, 2, 5}` | (any mode is decoded) |
| `mode-change-period=2` | `ModeChangePeriod: 2` | |
| `mode-change-neighbor=1` | `ModeChangeNeighbor: true` | |

`crc` and `robust-sorting` imply octet-aligned framing. With a mode set, `SetMode` and
`SetModeRequest` pick the highest allowed mode at or below the one given. With a
mode-change period or neighbor rule, `SetMode` sets a target, and the encoder reaches it
over the next frames as the rules allow. `interleaving` is not supported, so don't offer
or accept it.

### Mode requests (CMR)

Each payload carries a codec mode request: the mode its sender would like to receive.
`Decoder.ModeRequest` returns the peer's latest request, or `amrnb.NoModeRequest`, and
`Encoder.SetModeRequest` sets the one this side sends (none by default):

```go
dec.DecodeTo(pcm, pkt.Payload)
if m := dec.ModeRequest(); m != amrnb.NoModeRequest {
    enc.SetMode(m) // honour the peer's request; SetMode keeps to EncoderConfig.ModeSet
}
enc.SetModeRequest(amrnb.Mode0740) // ask the peer for 7.4 kbit/s
```

### DTX

With `EncoderConfig{DTX: true}` the encoder stops sending speech during silence. It sends
a short SID frame, usually every 8th frame, and an empty payload for the frames in between.
The sender has to skip those, while keeping the RTP clock running:

```go
payload, _ := enc.Encode(frames)
if len(payload) > 0 { // empty: NO_DATA, send nothing (RFC 4867, section 4.3.2)
    send(payload, timestamp, enc.Talkspurt()) // Talkspurt: the RTP marker bit
}
timestamp += uint32(len(frames)) // 160 per 20 ms frame
```

With several frames per packet, NO_DATA frames at the end of a payload are left out, so a
payload may carry fewer frames than the packet interval. As in `opencore-amrnb`, DTX is
chosen when the encoder is created and kept by `Reset`.

Any peer may use DTX, whether or not this encoder does, so a receiver has to run the decoder
every 20 ms, not only when a packet arrives. Otherwise each SID frame plays as a 20 ms burst
of noise instead of continuous comfort noise:

```go
if pkt != nil { // the packet due for this 20 ms interval
    dec.DecodeTo(pcm, pkt.Payload)
} else {
    dec.DecodeNoData(pcm) // comfort noise in DTX, concealment otherwise
}
```

With several frames per packet, call `DecodeNoData` for each 20 ms interval that the packet
covers but its payload doesn't (`DecodeTo` returns 160 samples per frame).

### `.amr` files

`FileWriter` stores RTP payloads in the single-channel AMR storage format (RFC 4867,
section 5), for example to record a call without transcoding. `FileReader` reads one back
as single-frame payloads:

```go
fw, _ := amrnb.NewFileWriter(f, amrnb.PayloadFormat{OctetAligned: true})
n, _ := fw.WritePayload(payload) // stores its n frames
fw.WriteNoData()                 // once per 20 ms not covered by a payload

fr, _ := amrnb.NewFileReader(f, amrnb.PayloadFormat{OctetAligned: true})
for {
    n, err := fr.ReadPayload(buf) // buf: amrnb.MaxPayloadBytes
    if err == io.EOF {
        break
    }
    if n == 0 {
        dec.DecodeNoData(pcm) // a NO_DATA frame
    } else {
        dec.DecodeTo(pcm, buf[:n])
    }
}
```

### Upgrading from the first release

The first release decoded only good speech frames, and encoded without DTX, CMR or
several frames per packet. Code written against it still compiles, but behaves
differently in these cases:

- **Bad and missing frames:** speech frames with Q=0, SPEECH_LOST and NO_DATA frames are
  concealed from the previous frames, and SID frames produce comfort noise. The first
  release returned silence for all of them.
- **Decoded speech:** good frames now go through code-gain averaging, phase dispersion and
  the rest of the reference decoder, so the PCM differs from the first release's, which
  was not bit-exact.
- **Encoded tones:** stationary tones (DTMF, ringback) are now coded with the reference's
  tone stabilization, so their frames differ from the first release's.
- **`Encode` input:** any positive multiple of 160 samples is accepted, giving one payload
  with that many frames. Before, anything but exactly 160 samples was an error.
- **Payload parsing:** frame types 9–13 are rejected, as before. Frame type 14 is now
  decoded as a lost frame.
- **New, all off by default:** `EncoderConfig.DTX`, `CRC`, `RobustSorting`, `ModeSet`,
  `ModeChangePeriod` and `ModeChangeNeighbor`, and `DecoderConfig.CRC` and `RobustSorting`.
  With none of them set, payloads are laid out as before: a CMR of 15 (none) and one frame
  each.

## Performance

Both directions run **far faster than real time** on a single core (a 20 ms frame's budget is
20 ms). The hot FIR/correlation kernels (autocorrelation, open-loop pitch, `cor_h_x`,
normalized correlation, residual, the adaptive-codebook interpolation `Pred_lt_3or6` and the
encoder's `Convolve`) use AVX2 assembly on `amd64` — `firRaw` (sliding correlation) and
`firDot` (horizontal dot product) — falling back to a bit-identical pure-Go path elsewhere
(or on `amd64` with `-tags purego`), which sums with four accumulators. Every kernel is
fuzz-tested bit-identical to its scalar fallback. Encode and decode do not allocate.

Measured on an **AMD Ryzen 9 7900 (Zen 4, AVX2), Go 1.26.3, linux/amd64, single core**.
`Go ×RT` = how many times faster than real time one core runs (so also ≈ concurrent streams
per core). `C` is the Apache-2.0 `opencore-amrnb` reference (Ubuntu 24.04's
`libopencore-amrnb0`), measured across a subprocess with the two-point-slope method (4000
frames) to cancel process startup; the residual per-frame pipe copy modestly favors the
in-process Go figure. Each figure is the median of three runs that time Go and C back to
back. **Caveat:** absolute figures depend on CPU boost state and system load, so treat
`Go ×RT` as order-of-magnitude. Deterministic synthetic voiced input. Reproduce with
`make bench` (Go-only) and `make bench-vs-c` (needs the C harnesses).

**Encode** (µs per 20 ms frame):

| Mode | Go | C | Go/C | Go ×RT |
|---|---|---|---|---|
| MR475 | 35.3 | 38.2 | 0.92 | 567× |
| MR515 | 27.4 | 30.1 | 0.92 | 731× |
| MR59  | 33.1 | 36.0 | 0.92 | 604× |
| MR67  | 44.7 | 46.5 | 0.97 | 447× |
| MR74  | 42.9 | 44.0 | 0.98 | 466× |
| MR795 | 48.6 | 46.9 | 1.04 | 411× |
| MR102 | 47.2 | 43.9 | 1.07 | 424× |
| MR122 | 45.2 | 42.4 | 1.11 | 442× |

**Decode** (µs per 20 ms frame):

| Mode | Go | C | Go/C | Go ×RT |
|---|---|---|---|---|
| MR475 | 7.7 | 8.6 | 0.90 | 2614× |
| MR515 | 7.7 | 8.7 | 0.89 | 2591× |
| MR59  | 7.7 | 8.6 | 0.89 | 2613× |
| MR67  | 8.0 | 8.8 | 0.91 | 2511× |
| MR74  | 7.2 | 8.9 | 0.87 | 2791× |
| MR795 | 8.1 | 9.0 | 0.90 | 2460× |
| MR102 | 7.3 | 8.6 | 0.87 | 2724× |
| MR122 | 7.3 | 7.9 | 0.93 | 2734× |

With `-tags purego` (the path every non-amd64 target uses) encode takes about 45–65 µs and
decode 8–11 µs per frame on the same machine (medians of three runs). Decode is 4.5–6×
faster than encode, which is
inherent to ACELP: the encoder *searches* for its parameters by analysis-by-synthesis
(open/closed-loop pitch, the algebraic pulse search, split-VQ codebooks), while the decoder
just *applies* them. Most of the remaining encode time is the codebook correlation matrix
(`cor_h`) and the pulse searches, which are scalar in the C reference too.

## Tests & benchmarks

```bash
# Unit tests (pure Go, no external dependencies)
go test .          # or: make test

# Go micro-benchmarks (encode/decode ns/op + xRT real-time factor, per mode)
go test -bench 'BenchmarkEncode|BenchmarkDecode' -benchmem .   # or: make bench
```

### Differential & speed-vs-C tests (optional, need the C reference)

The bit-exact differential tests and the Go-vs-C speed comparison drive the stock
Apache-2.0 `opencore-amrnb` shared library (encoder and decoder) through small harnesses
in `testdata/refharness`. They **skip** unless pointed at those harnesses. CI runs them on
amd64 and arm64.

```bash
# Debian/Ubuntu: the runtime library is enough (the harnesses declare the API).
sudo apt-get install libopencore-amrnb0

make harness   # builds testdata/refharness/bin/{amrnbdec,amrnbenc}
make diff      # all differential tests (decoder and encoder, via the public API)
GOFLAGS=-tags=purego make diff # the same, using the portable (non-AVX2) kernels
make bench-vs-c                # Go-vs-C speed comparison
# On macOS with Homebrew (brew install opencore-amr):
make harness AMRNB_LIBS="-L/opt/homebrew/lib -lopencore-amrnb"
```

The reference is Ubuntu 24.04's `libopencore-amrnb0`. Its comfort noise depends on how
`opencore-amrnb` was compiled: in `A_Refl`, which derives the prediction gain of the
comfort-noise filter, `pv_round` saturates through `L_add`, which is written as a plain
signed addition. Built with GCC and link-time optimization, as Debian and Ubuntu build it,
the compiler inlines it there, assumes the addition cannot overflow and drops the
saturation, so a value within 0x8000 of the maximum wraps instead. This package does the
same. A build without LTO keeps the saturation, so against it the comfort noise of some
SID frames differs; speech, concealment and the encoder are not affected.

The decoder test feeds identical streams to the public `Decoder` (both payload formats) and
to `Decoder_Interface_Decode`, and requires identical PCM. The streams cover random bits,
reference-encoded speech from quiet to clipped and pathological input, mode switches, lost
frames (including bursts and a lost first frame), DTX streams with and without losses, and
SID frames with random parameters in orders no encoder produces (SID_UPDATE without
SID_FIRST, long NO_DATA runs that mute the comfort noise, mode changes). The encoder test
compares speech, the pathological signal (full-scale tones, DTMF, square waves, impulses,
sweeps, noise) and a speech-and-noise signal for DTX, through the public `Encoder` in both
payload formats, with DTX off and on, and with mode switches up to every frame. Further
tests pack 2, 3 and 5 frames per payload in every payload format and require the
reference's frames in each one, and require a `FileWriter` file to match the reference's
`.amr` output byte for byte. To run a test directly, set `AMRNB_DIFF` (decoder harness) and
`AMRNB_ENC` (encoder harness) to the built binaries. `AMRNB_BENCH_FRAMES` tunes the speed
comparison's frame count (default 4000).

## License

Apache-2.0. This is a derivative of the Apache-2.0 `opencore-amrnb` reference — see
[NOTICE](NOTICE) for attribution and [LICENSE](LICENSE) for terms.
