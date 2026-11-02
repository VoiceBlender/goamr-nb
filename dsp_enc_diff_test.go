package amrnb

import (
	"bytes"
	"testing"
)

// TestEncDiffAgainstCReference encodes identical PCM with the public Encoder
// (both payload formats) and with the stock opencore-amrnb
// Encoder_Interface_Encode, and requires identical frames: speech from quiet
// to clipped, pathological input (tones, DTMF, square waves, noise), mode
// switches, and DTX on, where the reference's NO_DATA frames must be the
// Encoder's empty payloads. Set AMRNB_ENC (see make diff).
func TestEncDiffAgainstCReference(t *testing.T) {
	encHarness(t)
	speech, stress, dtx := testSignal(), stressSignal(), dtxSignal()
	checkDTXCoverage(t, refEncode(t, dtx, fixedMode(Mode0740), true))
	for m := Mode0475; m < numSpeechModes; m++ {
		name := benchModeNames[m]
		t.Run("speech/"+name, func(t *testing.T) { compareEncoders(t, speech, fixedMode(m), false) })
		t.Run("stress/"+name, func(t *testing.T) { compareEncoders(t, stress, fixedMode(m), false) })
		t.Run("dtx/"+name, func(t *testing.T) { compareEncoders(t, dtx, fixedMode(m), true) })
		t.Run("dtxstress/"+name, func(t *testing.T) { compareEncoders(t, stress, fixedMode(m), true) })
		t.Run("dtxspeech/"+name, func(t *testing.T) { compareEncoders(t, speech, fixedMode(m), true) })
	}
	t.Run("switch", func(t *testing.T) { compareEncoders(t, speech, cycleModes(7), false) })
	t.Run("switch/everyframe", func(t *testing.T) { compareEncoders(t, speech, cycleModes(1), false) })
	t.Run("switch/stress", func(t *testing.T) { compareEncoders(t, stress, cycleModes(3), false) })
	t.Run("dtxswitch", func(t *testing.T) { compareEncoders(t, dtx, cycleModes(5), true) })
	t.Run("dtxswitch/everyframe", func(t *testing.T) { compareEncoders(t, dtx, cycleModes(1), true) })
}

// compareEncoders encodes pcm with the reference and with the public Encoder
// in both payload formats, frame f in mode modeOf(f), and requires the same
// frames.
func compareEncoders(t *testing.T, pcm []int16, modeOf func(int) Mode, dtx bool) {
	t.Helper()
	want := refEncode(t, pcm, modeOf, dtx)
	for _, octet := range []bool{true, false} {
		enc, err := NewEncoder(EncoderConfig{Mode: modeOf(0), OctetAligned: octet, DTX: dtx})
		if err != nil {
			t.Fatal(err)
		}
		format := payloadFormat{octetAligned: octet}
		for i, rf := range want {
			enc.SetMode(modeOf(i))
			p, err := enc.Encode(pcm[i*FrameSamples : (i+1)*FrameSamples])
			if err != nil {
				t.Fatalf("frame %d: %v", i, err)
			}
			if refFT(rf) == FTNoData {
				if len(p) != 0 {
					t.Fatalf("octet=%v frame %d: payload of %d bytes, want none (NO_DATA)", octet, i, len(p))
				}
				continue
			}
			_, frames, err := unpackPayload(p, format)
			if err != nil || len(frames) != 1 {
				t.Fatalf("octet=%v frame %d: payload %x: %v", octet, i, p, err)
			}
			f := frames[0]
			if f.ft != refFT(rf) || !f.q || !bytes.Equal(f.data, rf[1:]) {
				t.Fatalf("octet=%v frame %d (mode %d): FT %d %x, want FT %d %x", octet, i, modeOf(i), f.ft, f.data, refFT(rf), rf[1:])
			}
		}
	}
	t.Logf("%d frames bit-exact", len(want))
}

// checkDTXCoverage requires a DTX stream to hold speech, SID_FIRST,
// SID_UPDATE and NO_DATA frames, so that the DTX tests exercise them all.
func checkDTXCoverage(t *testing.T, frames []refFrame) {
	t.Helper()
	var speech, first, update, noData int
	for _, f := range frames {
		switch ft := refFT(f); {
		case ft <= FTMode1220:
			speech++
		case ft == FTSID && f[1+35/8]&(0x80>>(35%8)) != 0:
			update++
		case ft == FTSID:
			first++
		case ft == FTNoData:
			noData++
		}
	}
	if speech == 0 || first == 0 || update == 0 || noData == 0 {
		t.Fatalf("DTX test signal: %d speech, %d SID_FIRST, %d SID_UPDATE, %d NO_DATA frames; want all", speech, first, update, noData)
	}
}
