package amrnb

import (
	"math"
	"testing"
)

// genVoiced builds a deterministic pseudo-voiced signal: a low-frequency
// "pitch" excitation shaped by a couple of formant-like sinusoids, amplitude
// well inside int16 so the fixed-point accumulators behave like real speech.
func genVoiced(n, seed int) []int16 {
	x := make([]int16, n)
	for i := 0; i < n; i++ {
		t := float64(i)
		pitch := 0.5 + 0.5*sinApprox(t*(0.045+0.004*float64(seed)))
		v := 6000*sinApprox(t*0.11)*pitch +
			2500*sinApprox(t*0.37) +
			1200*sinApprox(t*0.83)
		x[i] = int16(v)
	}
	return x
}

// energy returns the mean-square of a slice.
func energy(x []int16) float64 {
	var s float64
	for _, v := range x {
		s += float64(v) * float64(v)
	}
	return s / float64(len(x))
}

// TestEncodeDecodeRoundTrip drives the full encoder for every supported mode and
// confirms it produces a bitstream the decoder turns into bounded, non-silent
// speech that tracks the input energy. This exercises the whole cod_amr driver:
// LP analysis, LSP quant, open-loop pitch, the four-subframe A-by-S loop, gain
// quant, and serialization, plus the decoder end to end.
func TestEncodeDecodeRoundTrip(t *testing.T) {
	modes := []struct {
		name string
		m    Mode
	}{
		{"MR475", Mode0475},
		{"MR515", Mode0515},
		{"MR59", Mode0590},
		{"MR67", Mode0670},
		{"MR74", Mode0740},
		{"MR795", Mode0795},
		{"MR102", Mode1020},
		{"MR122", Mode1220},
	}
	const frames = 12
	for _, mc := range modes {
		t.Run(mc.name, func(t *testing.T) {
			var enc encoderState
			enc.reset()
			var dec decoderState
			dec.reset()

			var totalIn, totalOut float64
			for f := 0; f < frames; f++ {
				in := genVoiced(cL_FRAME, f)
				payload, err := enc.encodeFrame(mc.m, in)
				if err != nil {
					t.Fatalf("frame %d: encode: %v", f, err)
				}
				if len(payload) != int(mc.m.Bytes()) {
					t.Fatalf("frame %d: payload %d bytes, want %d", f, len(payload), mc.m.Bytes())
				}

				out, err := dec.decodeFrame(frame{ft: int(mc.m), q: true, data: payload})
				if err != nil {
					t.Fatalf("frame %d: decode: %v", f, err)
				}
				if len(out) != cL_FRAME {
					t.Fatalf("frame %d: decoded %d samples, want %d", f, len(out), cL_FRAME)
				}
				for i, v := range out {
					if v == math.MinInt16 {
						t.Fatalf("frame %d sample %d: pathological output", f, i)
					}
				}
				if f >= 2 { // skip warm-up (zero history + 40-sample lookahead)
					totalIn += energy(in)
					totalOut += energy(out)
				}
			}

			if totalOut == 0 {
				t.Fatalf("%s: decoder produced pure silence", mc.name)
			}
			// reconstructed energy should be within an order of magnitude of input.
			ratio := totalOut / totalIn
			if ratio < 0.05 || ratio > 20 {
				t.Errorf("%s: energy ratio out/in = %.3f (in=%.0f out=%.0f)", mc.name, ratio, totalIn, totalOut)
			}
		})
	}
}
