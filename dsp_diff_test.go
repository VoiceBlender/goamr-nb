package amrnb

import (
	"bytes"
	"encoding/binary"
	"os"
	"os/exec"
	"strconv"
	"testing"
)

// TestDiffAgainstCReference decodes identical AMR-NB frames with this Go port
// and the Apache-2.0 opencore-amrnb C reference, asserting bit-exact PCM. Set
// AMRNB_DIFF to the compiled reference harness: it reads the per-frame packed
// speech bytes on stdin for the mode given as argv[1] (ceil(bits/8) bytes/frame,
// no ToC) and writes 160 int16 LE/frame to stdout. The harness itself is built
// separately (out of scope here); this test SKIPS until AMRNB_DIFF is set.
//
// Both decoders truncate the output to 13 bits (decodeSpeechFrame does the same
// &^7 as the reference), so the comparison is exact — no extra masking.
func TestDiffAgainstCReference(t *testing.T) {
	bin := os.Getenv("AMRNB_DIFF")
	if bin == "" {
		t.Skip("set AMRNB_DIFF to the opencore-amrnb reference decoder harness to run")
	}

	for mode := Mode0475; mode < numSpeechModes; mode++ {
		n := mode.Bytes()
		nb := modeBits[mode]
		const frames = 30

		// Deterministic pseudo-random frames; zero the padding bits past nbBits so
		// both decoders see exactly the same meaningful bitstream.
		seed := uint32(0x12345 + uint32(mode)*7)
		raw := make([]byte, n*frames)
		for i := range raw {
			seed = seed*1664525 + 1013904223
			raw[i] = byte(seed >> 16)
		}
		for f := 0; f < frames; f++ {
			base := f * n
			for bit := nb; bit < n*8; bit++ {
				raw[base+bit/8] &^= 1 << uint(7-bit%8)
			}
		}

		// C reference.
		cmd := exec.Command(bin, strconv.Itoa(int(mode)))
		cmd.Stdin = bytes.NewReader(raw)
		var cout bytes.Buffer
		cmd.Stdout = &cout
		if err := cmd.Run(); err != nil {
			t.Fatalf("mode %d: C harness: %v", mode, err)
		}
		cpcm := make([]int16, cout.Len()/2)
		binary.Read(&cout, binary.LittleEndian, &cpcm)
		if len(cpcm) != frames*FrameSamples {
			t.Fatalf("mode %d: C produced %d samples, want %d", mode, len(cpcm), frames*FrameSamples)
		}

		// Go decode of the same frames.
		var st decoderState
		st.reset()
		gpcm := make([]int16, 0, frames*FrameSamples)
		for f := 0; f < frames; f++ {
			out, err := st.decodeFrame(frame{ft: int(mode), q: true, data: raw[f*n : f*n+n]})
			if err != nil {
				t.Fatalf("mode %d frame %d: Go decode: %v", mode, f, err)
			}
			gpcm = append(gpcm, out...)
		}

		mism, firstAt, maxDiff := 0, -1, 0
		for i := range cpcm {
			d := int(gpcm[i]) - int(cpcm[i])
			if d != 0 {
				mism++
				if firstAt < 0 {
					firstAt = i
				}
				if d < 0 {
					d = -d
				}
				if d > maxDiff {
					maxDiff = d
				}
			}
		}
		if mism != 0 {
			t.Errorf("mode %d: %d/%d differ, maxDiff=%d (first %d: go=%d c=%d)",
				mode, mism, len(cpcm), maxDiff, firstAt, gpcm[firstAt], cpcm[firstAt])
		} else {
			t.Logf("mode %d: bit-exact over %d frames", mode, frames)
		}
	}
}
