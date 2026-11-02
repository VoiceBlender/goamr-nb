package amrnb

import "testing"

// lcgBytes fills n bytes from a deterministic LCG seeded by seed.
func lcgBytes(seed uint32, n int) []byte {
	b := make([]byte, n)
	for i := range b {
		seed = seed*1664525 + 1013904223
		b[i] = byte(seed >> 16)
	}
	return b
}

// TestDecodeAllModesRuns drives every speech mode through the full decode
// pipeline (parameter decode + post-filter + HP) over several frames of
// deterministic pseudo-random bitstream and checks it produces the right number
// of bounded, eventually non-silent samples without panicking.
func TestDecodeAllModesRuns(t *testing.T) {
	for m := Mode0475; m < numSpeechModes; m++ {
		d := NewDecoder(DecoderConfig{OctetAligned: true})
		var energy float64
		nb := m.Bytes()
		for f := 0; f < 30; f++ {
			data := lcgBytes(uint32(0x1000+int(m)*97+f*7), nb)
			fr := frame{ft: m.FrameType(), q: true, data: data}
			payload := packPayloadOctet(noCMR, []frame{fr})
			pcm, err := d.Decode(payload)
			if err != nil {
				t.Fatalf("mode %d frame %d: %v", m, f, err)
			}
			if len(pcm) != FrameSamples {
				t.Fatalf("mode %d: decoded %d samples, want %d", m, len(pcm), FrameSamples)
			}
			for _, s := range pcm {
				energy += float64(s) * float64(s)
				if s&7 != 0 {
					t.Fatalf("mode %d: output not 13-bit truncated: %d", m, s)
				}
			}
		}
		if energy == 0 {
			t.Errorf("mode %d: decoder produced only silence over 30 frames", m)
		}
	}
}

// TestDecodeDeterministic checks two independent decoders produce identical
// output for the same input sequence.
func TestDecodeDeterministic(t *testing.T) {
	mode := Mode0590
	nb := mode.Bytes()
	d1 := NewDecoder(DecoderConfig{OctetAligned: true})
	d2 := NewDecoder(DecoderConfig{OctetAligned: true})
	for f := 0; f < 20; f++ {
		data := lcgBytes(uint32(0x55+f*13), nb)
		fr := frame{ft: mode.FrameType(), q: true, data: data}
		payload := packPayloadOctet(noCMR, []frame{fr})
		p1, _ := d1.Decode(payload)
		p2, _ := d2.Decode(payload)
		for i := range p1 {
			if p1[i] != p2[i] {
				t.Fatalf("frame %d sample %d: %d != %d", f, i, p1[i], p2[i])
			}
		}
	}
}

// TestDecodeStateContinuity checks the decoder carries inter-frame state: the
// same payload decoded as the 2nd frame differs from decoding it cold.
func TestDecodeStateContinuity(t *testing.T) {
	mode := Mode0670
	nb := mode.Bytes()
	mk := func(seed uint32) []byte {
		fr := frame{ft: mode.FrameType(), q: true, data: lcgBytes(seed, nb)}
		return packPayloadOctet(noCMR, []frame{fr})
	}
	warm := NewDecoder(DecoderConfig{OctetAligned: true})
	warm.Decode(mk(1)) // prime state
	warmOut, _ := warm.Decode(mk(2))

	cold := NewDecoder(DecoderConfig{OctetAligned: true})
	coldOut, _ := cold.Decode(mk(2))

	differs := false
	for i := range warmOut {
		if warmOut[i] != coldOut[i] {
			differs = true
			break
		}
	}
	if !differs {
		t.Error("decoder ignored carried inter-frame state")
	}
}

// TestDecodeBadFrameConceals checks that a speech frame with Q=0 is concealed
// from the previous frames rather than decoded from its bits: the output
// continues the signal and differs from decoding the same bits with Q=1.
func TestDecodeBadFrameConceals(t *testing.T) {
	enc, _ := NewEncoder(EncoderConfig{Mode: Mode0590, OctetAligned: true})
	frames := benchGenFrames(12)
	var payloads [][]byte
	for _, f := range frames {
		p, err := enc.Encode(f)
		if err != nil {
			t.Fatal(err)
		}
		payloads = append(payloads, p)
	}
	good := NewDecoder(DecoderConfig{OctetAligned: true})
	bad := NewDecoder(DecoderConfig{OctetAligned: true})
	for _, p := range payloads[:11] {
		good.Decode(p)
		bad.Decode(p)
	}
	last := append([]byte(nil), payloads[11]...)
	want, _ := good.Decode(last)
	last[1] &^= 0x04 // clear the Q bit
	got, err := bad.Decode(last)
	if err != nil {
		t.Fatal(err)
	}
	var e float64
	same := true
	for i := range got {
		e += float64(got[i]) * float64(got[i])
		same = same && got[i] == want[i]
	}
	if same {
		t.Fatal("Q=0 frame decoded like a good frame")
	}
	if e == 0 {
		t.Fatal("Q=0 frame decoded to silence, want concealment")
	}
}
