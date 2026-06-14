package amrnb

import (
	"bytes"
	"encoding/binary"
	"os"
	"os/exec"
	"strconv"
	"testing"
)

// TestEncDiffAgainstCReference encodes identical PCM frames with this Go encoder
// and the Apache-2.0 opencore-amrnb C reference, comparing the packed bitstream.
// Set AMRNB_ENC to the compiled reference harness: it reads 160 int16 LE/frame
// on stdin for the mode given as argv[1] and writes storage-format frames (1 ToC
// byte + ceil(bits/8) packed speech bytes each). The harness is built separately
// (out of scope here); this test SKIPS until AMRNB_ENC is set.
func TestEncDiffAgainstCReference(t *testing.T) {
	bin := os.Getenv("AMRNB_ENC")
	if bin == "" {
		t.Skip("set AMRNB_ENC to the opencore-amrnb reference encoder harness to run")
	}
	const frames = 20

	// Deterministic speech-like input (sum of tones), matching the benchmark
	// generator's shape so the encoder exercises real pitch/codebook paths.
	raw := new(bytes.Buffer)
	pcm := make([][]int16, frames)
	phase := 0.0
	for f := 0; f < frames; f++ {
		s := make([]int16, FrameSamples)
		for i := range s {
			s[i] = int16(5000*sinApprox(phase) + 2000*sinApprox(phase*2.7))
			phase += 2 * 3.14159265 * 300 / SampleRate
		}
		pcm[f] = s
		binary.Write(raw, binary.LittleEndian, s)
	}

	for mode := Mode0475; mode < numSpeechModes; mode++ {
		t.Run(benchModeNames[mode], func(t *testing.T) {
			packed := mode.Bytes() // speech bytes, excluding ToC
			frameLen := 1 + packed

			cmd := exec.Command(bin, strconv.Itoa(int(mode)))
			cmd.Stdin = bytes.NewReader(raw.Bytes())
			var cout bytes.Buffer
			cmd.Stdout = &cout
			if err := cmd.Run(); err != nil {
				t.Fatalf("C encoder harness: %v", err)
			}
			cbytes := cout.Bytes()
			if len(cbytes) != frames*frameLen {
				t.Fatalf("C produced %d bytes, want %d", len(cbytes), frames*frameLen)
			}

			var enc encoderState
			enc.reset()
			for f := 0; f < frames; f++ {
				goPacked, err := enc.encodeFrame(mode, pcm[f])
				if err != nil {
					t.Fatalf("frame %d: Go encode: %v", f, err)
				}
				cPacked := cbytes[f*frameLen+1 : (f+1)*frameLen] // skip ToC byte
				if bytes.Equal(goPacked, cPacked) {
					continue
				}
				// Localize: first differing internal parameter (mimeUnsort yields
				// the parameter array in decoder-consumption order).
				gp := mimeUnsort(goPacked, mode)
				cp := mimeUnsort(cPacked, mode)
				first := -1
				for i := range gp {
					if gp[i] != cp[i] {
						first = i
						break
					}
				}
				field := "param" + strconv.Itoa(first)
				if mode == Mode0475 {
					field = fieldOfMR475(first)
				}
				t.Fatalf("frame %d: diverges at internal param %d (%s): go=%d c=%d",
					f, first, field, gp[first], cp[first])
			}
			t.Logf("mode %d: all %d frames bit-exact", mode, frames)
		})
	}
}

// fieldOfMR475 names an MR475 internal parameter index: LSF(3) then four
// subframes of [pitch, cb-pos, cb-sign], with the joint gain index appended
// after the codebook of each even subframe.
func fieldOfMR475(i int) string {
	if i < 0 {
		return "?"
	}
	if i < 3 {
		return "LSF" + strconv.Itoa(i)
	}
	p := i - 3
	for sf := 0; sf < 4; sf++ {
		even := sf%2 == 0
		blk := 3 // pitch + cb-pos + cb-sign
		if even {
			blk = 4 // + joint gain
		}
		if p < blk {
			switch p {
			case 0:
				return "sf" + strconv.Itoa(sf) + ".pitch"
			case 1:
				return "sf" + strconv.Itoa(sf) + ".cbPos"
			case 2:
				return "sf" + strconv.Itoa(sf) + ".cbSign"
			default:
				return "sf" + strconv.Itoa(sf) + ".gain"
			}
		}
		p -= blk
	}
	return "?tail"
}
