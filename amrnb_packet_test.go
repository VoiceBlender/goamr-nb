package amrnb

import (
	"bytes"
	"slices"
	"testing"
)

// TestEncMultiFrameDiffAgainstCReference encodes k frames per payload with
// the public Encoder, in every payload format, and requires the frames of
// opencore-amrnb's Encoder_Interface_Encode (AMRNB_ENC) in each payload: trailing NO_DATA
// frames left out, no payload when all are NO_DATA, and the marker bit only
// when the first frame starts a talkspurt.
func TestEncMultiFrameDiffAgainstCReference(t *testing.T) {
	encHarness(t)
	for _, c := range []struct {
		name   string
		sig    []int16
		modeOf func(int) Mode
		dtx    bool
	}{
		// Modes change every 30 frames, at a group boundary for every k.
		{"dtx", dtxSignal(), func(f int) Mode { return Mode((f / 30) % 8) }, true},
		{"speech", testSignal(), func(f int) Mode { return Mode((f/30 + 4) % 8) }, false},
	} {
		ref := refEncode(t, c.sig, c.modeOf, c.dtx)
		for _, f := range allFormats {
			for _, k := range []int{2, 3, 5} {
				t.Run(c.name+"/"+f.name+"/"+string(rune('0'+k)), func(t *testing.T) {
					multiFrameCompare(t, c.sig, ref, c.modeOf, c.dtx, f.pf, k)
				})
			}
		}
	}
}

func multiFrameCompare(t *testing.T, sig []int16, ref []refFrame, modeOf func(int) Mode, dtx bool, pf payloadFormat, k int) {
	t.Helper()
	enc, err := NewEncoder(EncoderConfig{Mode: modeOf(0), DTX: dtx, OctetAligned: pf.octetAligned, CRC: pf.crc, RobustSorting: pf.robustSorting})
	if err != nil {
		t.Fatal(err)
	}
	var pp payloadParser
	dst := make([]byte, MaxPayloadBytesFor(k))
	for g := 0; (g+1)*k <= len(ref); g++ {
		mode := modeOf(g * k)
		if modeOf((g+1)*k-1) != mode {
			t.Fatalf("group %d spans a mode switch", g)
		}
		enc.SetMode(mode)
		n, err := enc.EncodeTo(dst, sig[g*k*FrameSamples:(g+1)*k*FrameSamples])
		if err != nil {
			t.Fatalf("group %d: %v", g, err)
		}
		want := ref[g*k : (g+1)*k]
		for len(want) > 0 && refFT(want[len(want)-1]) == FTNoData {
			want = want[:len(want)-1]
		}
		if len(want) == 0 {
			if n != 0 {
				t.Fatalf("group %d: %d bytes for an all-NO_DATA group", g, n)
			}
			continue
		}
		frames, err := pp.parse(dst[:n], pf)
		if err != nil {
			t.Fatalf("group %d: parse: %v", g, err)
		}
		if len(frames) != len(want) {
			t.Fatalf("group %d: %d frames, want %d", g, len(frames), len(want))
		}
		for i, f := range frames {
			if f.ft != refFT(want[i]) || !f.q || !bytes.Equal(f.data, want[i][1:]) {
				t.Fatalf("group %d frame %d: ft %d data %x, want ft %d data %x", g, i, f.ft, f.data, refFT(want[i]), want[i][1:])
			}
		}
		first := g * k
		if wantM := refFT(ref[first]) <= FTMode1220 && (first == 0 || refFT(ref[first-1]) > FTMode1220); enc.Talkspurt() != wantM {
			t.Fatalf("group %d: Talkspurt = %v, want %v", g, enc.Talkspurt(), wantM)
		}
	}
}

// TestMultiFrameMatchesSingleFrame checks, without the C reference, that k
// frames per payload carry exactly the frames of single-frame payloads, and
// that every decoder format decodes them to the same PCM.
func TestMultiFrameMatchesSingleFrame(t *testing.T) {
	sig := dtxSignal()
	const k = 3
	for _, f := range allFormats {
		cfg := EncoderConfig{Mode: Mode0740, DTX: true, OctetAligned: f.pf.octetAligned, CRC: f.pf.crc, RobustSorting: f.pf.robustSorting}
		one, _ := NewEncoder(EncoderConfig{Mode: Mode0740, DTX: true, OctetAligned: true})
		many, _ := NewEncoder(cfg)
		decOne := NewDecoder(DecoderConfig{OctetAligned: true})
		decMany := NewDecoder(DecoderConfig{OctetAligned: f.pf.octetAligned, CRC: f.pf.crc, RobustSorting: f.pf.robustSorting})
		var pp payloadParser
		pcmOne := make([]int16, k*FrameSamples)
		pcmMany := make([]int16, k*FrameSamples)
		for g := 0; (g+1)*k*FrameSamples <= len(sig); g++ {
			in := sig[g*k*FrameSamples : (g+1)*k*FrameSamples]
			var want []frame
			var marker bool
			for i := 0; i < k; i++ {
				p, err := one.Encode(in[i*FrameSamples : (i+1)*FrameSamples])
				if err != nil {
					t.Fatal(err)
				}
				if i == 0 {
					marker = one.Talkspurt()
				}
				if len(p) == 0 {
					want = append(want, frame{ft: FTNoData, q: true})
					decOne.DecodeNoData(pcmOne[i*FrameSamples:])
					continue
				}
				fr, _ := pp.parse(p, payloadFormat{octetAligned: true})
				want = append(want, frame{ft: fr[0].ft, q: fr[0].q, data: slices.Clone(fr[0].data)})
				decOne.DecodeTo(pcmOne[i*FrameSamples:], p)
			}
			for len(want) > 0 && want[len(want)-1].ft == FTNoData {
				want = want[:len(want)-1]
			}
			p, err := many.Encode(in)
			if err != nil {
				t.Fatal(err)
			}
			if many.Talkspurt() != marker {
				t.Fatalf("%s group %d: Talkspurt = %v, want %v", f.name, g, many.Talkspurt(), marker)
			}
			if len(want) == 0 {
				if len(p) != 0 {
					t.Fatalf("%s group %d: payload for all-NO_DATA frames", f.name, g)
				}
			} else {
				_, got, err := unpackPayload(p, f.pf)
				if err != nil {
					t.Fatal(err)
				}
				framesEqual(t, got, want, f.name)
			}
			n := 0
			if len(p) > 0 {
				if n, err = decMany.DecodeTo(pcmMany, p); err != nil {
					t.Fatal(err)
				}
			}
			for ; n < k*FrameSamples; n += FrameSamples {
				decMany.DecodeNoData(pcmMany[n:])
			}
			if !slices.Equal(pcmOne, pcmMany) {
				t.Fatalf("%s group %d: decoded PCM differs", f.name, g)
			}
		}
	}
}

func TestEncodeRejectsPartialFrames(t *testing.T) {
	enc, _ := NewEncoder(EncoderConfig{Mode: Mode0740})
	for _, n := range []int{0, 319, 321, 639} {
		if _, err := enc.Encode(make([]int16, n)); err == nil {
			t.Errorf("%d samples encoded", n)
		}
	}
	dst := make([]byte, MaxPayloadBytesFor(2)-1)
	enc2, _ := NewEncoder(EncoderConfig{Mode: Mode1220, OctetAligned: true, CRC: true})
	if _, err := enc2.EncodeTo(dst[:MaxPayloadBytes-1], make([]int16, FrameSamples)); err == nil {
		t.Error("EncodeTo accepted a buffer too small for a frame with CRC")
	}
	if n, err := enc2.EncodeTo(dst[:MaxPayloadBytes], make([]int16, FrameSamples)); err != nil || n != MaxPayloadBytes {
		t.Errorf("23.85 with CRC: %d bytes, %v; want %d", n, err, MaxPayloadBytes)
	}
}

func TestMultiFrameEncodeToDoesNotAllocate(t *testing.T) {
	sig := testSignal()[:3*FrameSamples]
	enc, _ := NewEncoder(EncoderConfig{Mode: Mode0740, OctetAligned: true, CRC: true, RobustSorting: true})
	dec := NewDecoder(DecoderConfig{CRC: true, RobustSorting: true})
	dst := make([]byte, MaxPayloadBytesFor(3))
	pcm := make([]int16, 3*FrameSamples)
	if allocs := testing.AllocsPerRun(20, func() {
		n, _ := enc.EncodeTo(dst, sig)
		dec.DecodeTo(pcm, dst[:n])
	}); allocs != 0 {
		t.Fatalf("multi-frame EncodeTo+DecodeTo allocates %.1f times", allocs)
	}
}

// encodeModes encodes n frames and returns the frame type of each.
func encodeModes(t *testing.T, enc *Encoder, n int, before func(i int)) []int {
	t.Helper()
	sig := testSignal()
	var fts []int
	for i := 0; i < n; i++ {
		if before != nil {
			before(i)
		}
		p, err := enc.Encode(sig[(100+i)*FrameSamples : (101+i)*FrameSamples])
		if err != nil {
			t.Fatal(err)
		}
		_, fr, err := unpackPayload(p, enc.format)
		if err != nil {
			t.Fatal(err)
		}
		fts = append(fts, fr[0].ft)
	}
	return fts
}

func TestModeSet(t *testing.T) {
	if _, err := NewEncoder(EncoderConfig{Mode: Mode0740, ModeSet: []Mode{Mode0475, Mode1220}}); err == nil {
		t.Error("mode outside the mode set accepted")
	}
	if _, err := NewEncoder(EncoderConfig{Mode: Mode0475, ModeSet: []Mode{Mode0475, 9}}); err == nil {
		t.Error("invalid mode in the mode set accepted")
	}
	if _, err := NewEncoder(EncoderConfig{Mode: Mode0475, ModeChangePeriod: 3}); err == nil {
		t.Error("mode change period 3 accepted")
	}
	enc, err := NewEncoder(EncoderConfig{Mode: Mode0740, OctetAligned: true, ModeSet: []Mode{Mode0515, Mode0740, Mode1020}})
	if err != nil {
		t.Fatal(err)
	}
	set := []Mode{Mode1220, Mode0590, Mode0475, Mode1020, Mode0515}
	fts := encodeModes(t, enc, len(set), func(i int) { enc.SetMode(set[i]) })
	if want := []int{FTMode1020, FTMode0515, FTMode0515, FTMode1020, FTMode0515}; !slices.Equal(fts, want) {
		t.Errorf("modes %v, want %v", fts, want)
	}
	dec := NewDecoder(DecoderConfig{OctetAligned: true})
	for _, c := range []struct{ req, want Mode }{{Mode1220, Mode1020}, {Mode0475, Mode0515}, {Mode0740, Mode0740}, {NoModeRequest, NoModeRequest}} {
		enc.SetModeRequest(c.req)
		p, _ := enc.Encode(make([]int16, FrameSamples))
		dec.Decode(p)
		if got := dec.ModeRequest(); got != c.want {
			t.Errorf("request %d sent as %d, want %d", c.req, got, c.want)
		}
	}
}

func TestModeChangePeriodAndNeighbor(t *testing.T) {
	// Period 2: changes only before even frames.
	enc, _ := NewEncoder(EncoderConfig{Mode: Mode0475, ModeChangePeriod: 2})
	fts := encodeModes(t, enc, 6, func(i int) {
		switch i {
		case 1:
			enc.SetMode(Mode1220) // due at frame 2
		case 2:
			enc.SetMode(Mode0740) // replaces it before it takes effect
		case 3:
			enc.SetMode(Mode0515) // due at frame 4
		}
	})
	if want := []int{0, 0, 4, 4, 1, 1}; !slices.Equal(fts, want) {
		t.Errorf("period 2: modes %v, want %v", fts, want)
	}
	// Neighbor: one step through the mode set per frame.
	enc, _ = NewEncoder(EncoderConfig{Mode: Mode0475, ModeSet: []Mode{Mode0475, Mode0740, Mode1020, Mode1220}, ModeChangeNeighbor: true})
	fts = encodeModes(t, enc, 7, func(i int) {
		switch i {
		case 0:
			enc.SetMode(Mode1220)
		case 4:
			enc.SetMode(Mode0475)
		}
	})
	if want := []int{4, 6, 7, 7, 6, 4, 0}; !slices.Equal(fts, want) {
		t.Errorf("neighbor: modes %v, want %v", fts, want)
	}
	// Both: one step every second frame.
	enc, _ = NewEncoder(EncoderConfig{Mode: Mode0475, ModeSet: []Mode{Mode0475, Mode0740, Mode1220}, ModeChangePeriod: 2, ModeChangeNeighbor: true})
	fts = encodeModes(t, enc, 6, func(i int) {
		if i == 0 {
			enc.SetMode(Mode1220)
		}
	})
	if want := []int{4, 4, 7, 7, 7, 7}; !slices.Equal(fts, want) {
		t.Errorf("period 2 + neighbor: modes %v, want %v", fts, want)
	}
}
