package amrnb

import (
	"testing"
)

// TestDecDiffAgainstCReference feeds identical streams to the public Decoder
// (both payload formats) and to the stock opencore-amrnb Decoder_Interface_Decode,
// and requires identical PCM. Set AMRNB_DIFF and AMRNB_ENC (see make diff).
func TestDecDiffAgainstCReference(t *testing.T) {
	decHarness(t)
	encHarness(t)
	speech, stress, dtx := testSignal(), stressSignal(), dtxSignal()
	for m := Mode0475; m < numSpeechModes; m++ {
		name := benchModeNames[m]
		t.Run("random/"+name, func(t *testing.T) { compareDecoders(t, randomFrames(m, 300, uint32(m)+1)) })
		t.Run("speech/"+name, func(t *testing.T) { compareDecoders(t, refEncode(t, speech, fixedMode(m), false)) })
		t.Run("stress/"+name, func(t *testing.T) { compareDecoders(t, refEncode(t, stress, fixedMode(m), false)) })
		t.Run("loss/"+name, func(t *testing.T) {
			compareDecoders(t, dropFrames(refEncode(t, speech, fixedMode(m), false), uint32(m)+11))
		})
		t.Run("dtx/"+name, func(t *testing.T) { compareDecoders(t, refEncode(t, dtx, fixedMode(m), true)) })
		t.Run("dtxloss/"+name, func(t *testing.T) {
			compareDecoders(t, dropFrames(refEncode(t, dtx, fixedMode(m), true), uint32(m)+23))
		})
		t.Run("sid/"+name, func(t *testing.T) { compareDecoders(t, sidScenarios(t, m, speech)) })
	}
	t.Run("switch", func(t *testing.T) { compareDecoders(t, refEncode(t, speech, cycleModes(7), false)) })
	t.Run("switchloss", func(t *testing.T) {
		compareDecoders(t, dropFrames(refEncode(t, speech, cycleModes(3), false), 99))
	})
	t.Run("dtxswitch", func(t *testing.T) { compareDecoders(t, refEncode(t, dtx, cycleModes(5), true)) })
	t.Run("random/mixed", func(t *testing.T) {
		var frames []refFrame
		for i := 0; i < 40; i++ {
			frames = append(frames, randomFrames(Mode((i*5)%8), 10, uint32(i)+100)...)
		}
		compareDecoders(t, frames)
	})
}

// compareDecoders decodes frames with the reference and with the public
// Decoder in both payload formats, and requires identical PCM.
func compareDecoders(t *testing.T, frames []refFrame) {
	t.Helper()
	want := refDecode(t, frames)
	for _, octet := range []bool{true, false} {
		dec := NewDecoder(DecoderConfig{OctetAligned: octet})
		for i, rf := range frames {
			f := frame{ft: refFT(rf), q: rf[0]&0x04 != 0, data: rf[1:]}
			var p []byte
			if octet {
				p = packPayloadOctet(noCMR, []frame{f})
			} else {
				p = packPayloadBandwidthEfficient(noCMR, []frame{f})
			}
			got, err := dec.Decode(p)
			if err != nil {
				t.Fatalf("octet=%v frame %d (FT %d): %v", octet, i, f.ft, err)
			}
			ref := want[i*FrameSamples : (i+1)*FrameSamples]
			for j := range ref {
				if got[j] != ref[j] {
					t.Fatalf("octet=%v frame %d (FT %d) sample %d: go=%d c=%d", octet, i, f.ft, j, got[j], ref[j])
				}
			}
		}
	}
	t.Logf("%d frames bit-exact", len(frames))
}

// randomFrames returns n speech frames of mode m with random bits.
func randomFrames(m Mode, n int, seed uint32) []refFrame {
	s := lcg(seed)
	frames := make([]refFrame, n)
	for i := range frames {
		f := make(refFrame, 1+m.Bytes())
		f[0] = byte(m)<<3 | 0x04
		for j := 1; j < len(f); j++ {
			f[j] = byte(s.next() >> 16)
		}
		frames[i] = f
	}
	return frames
}

var noDataFrame = refFrame{FTNoData<<3 | 0x04}

// dropFrames replaces frames with NO_DATA: the first frame, scattered single
// frames, and bursts of up to 8.
func dropFrames(frames []refFrame, seed uint32) []refFrame {
	s := lcg(seed)
	out := append([]refFrame(nil), frames...)
	out[0] = noDataFrame
	for i := 1; i < len(out); i++ {
		switch r := s.next() >> 24; {
		case r < 12:
			out[i] = noDataFrame
		case r < 15:
			for n := int(s.next()>>29) + 1; n > 0 && i < len(out); n-- {
				out[i] = noDataFrame
				i++
			}
		}
	}
	return out
}

// sidFrame returns a SID frame with random comfort-noise parameters, the STI
// bit (SID_UPDATE if sti, else SID_FIRST) and the mode indication m.
func sidFrame(s *lcg, sti bool, m Mode) refFrame {
	f := make(refFrame, 6)
	f[0] = FTSID<<3 | 0x04
	for j := 1; j < 6; j++ {
		f[j] = byte(s.next() >> 16)
	}
	// Bits 35 (STI) and 36..38 (mode, LSB first) of the 39 SID bits.
	setBit := func(i int, v bool) {
		if v {
			f[1+i/8] |= 0x80 >> (i % 8)
		} else {
			f[1+i/8] &^= 0x80 >> (i % 8)
		}
	}
	setBit(35, sti)
	for i := 0; i < 3; i++ {
		setBit(36+i, m>>i&1 != 0)
	}
	return f
}

// sidScenarios mixes speech with SID frames in orders no encoder produces:
// SID_UPDATE without SID_FIRST, back-to-back SID_FIRST, long NO_DATA runs
// (comfort-noise muting), and SID frames whose mode differs from the speech.
func sidScenarios(t *testing.T, m Mode, speech []int16) []refFrame {
	s := lcg(uint32(m) + 77)
	sp := refEncode(t, speech[:len(speech)/4], fixedMode(m), false)
	var out []refFrame
	add := func(f ...refFrame) { out = append(out, f...) }
	add(sp[:30]...)
	add(sidFrame(&s, false, m))
	for i := 0; i < 20; i++ {
		if i%8 == 2 {
			add(sidFrame(&s, true, m))
		} else {
			add(noDataFrame)
		}
	}
	add(sp[30:60]...)
	add(sidFrame(&s, true, m)) // SID_UPDATE straight after speech
	add(sidFrame(&s, true, (m+3)%8))
	add(sidFrame(&s, false, m), sidFrame(&s, false, m))
	for i := 0; i < 80; i++ { // long NO_DATA run
		add(noDataFrame)
	}
	add(sidFrame(&s, true, m))
	add(sp[60:70]...)
	add(noDataFrame, noDataFrame)
	add(sp[70:90]...)
	for i := 0; i < 30; i++ {
		add(sidFrame(&s, s.next()>>31 == 1, Mode(s.next()>>29)))
	}
	add(sp[90:]...)
	return out
}
