package amrnb

import "testing"

func TestNewEncoderValidatesMode(t *testing.T) {
	if _, err := NewEncoder(EncoderConfig{Mode: Mode(99)}); err == nil {
		t.Error("expected error for invalid mode")
	}
	if _, err := NewEncoder(EncoderConfig{Mode: Mode0670, OctetAligned: true}); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestEncodeRejectsWrongFrameSize(t *testing.T) {
	e, _ := NewEncoder(EncoderConfig{Mode: Mode0670, OctetAligned: true})
	if _, err := e.Encode(make([]int16, 100)); err == nil {
		t.Error("expected error for wrong sample count")
	}
}

// Encode produces an RFC 4867 payload of the correct length for every mode.
// (The DSP is stubbed for now, so payload bytes are zero — length is the gate.)
func TestEncodeProducesCorrectLength(t *testing.T) {
	samples := make([]int16, FrameSamples)
	for i := range samples {
		samples[i] = int16(2000 * sinApprox(float64(i)*0.2))
	}
	for m := Mode0475; m < numSpeechModes; m++ {
		e, err := NewEncoder(EncoderConfig{Mode: m, OctetAligned: true})
		if err != nil {
			t.Fatalf("mode %d: NewEncoder: %v", m, err)
		}
		payload, err := e.Encode(samples)
		if err != nil {
			t.Fatalf("mode %d: Encode: %v", m, err)
		}
		// octet-aligned: CMR + ToC + ceil(bits/8) speech bytes.
		if want := 1 + 1 + m.Bytes(); len(payload) != want {
			t.Errorf("mode %d: payload len %d, want %d", m, len(payload), want)
		}
	}
}

// Round-trip every mode through encode->decode and verify the decoder yields
// exactly one 160-sample frame and does not error. (Output content is silence
// until the DSP is ported.)
func TestEncodeDecodeRoundTripShape(t *testing.T) {
	samples := make([]int16, FrameSamples)
	for i := range samples {
		samples[i] = int16(3000 * sinApprox(float64(i)*0.15))
	}
	for _, octet := range []bool{true, false} {
		for m := Mode0475; m < numSpeechModes; m++ {
			e, _ := NewEncoder(EncoderConfig{Mode: m, OctetAligned: octet})
			d := NewDecoder(DecoderConfig{OctetAligned: octet})
			payload, err := e.Encode(samples)
			if err != nil {
				t.Fatalf("mode %d octet=%v: Encode: %v", m, octet, err)
			}
			pcm, err := d.Decode(payload)
			if err != nil {
				t.Fatalf("mode %d octet=%v: Decode: %v", m, octet, err)
			}
			if len(pcm) != FrameSamples {
				t.Errorf("mode %d octet=%v: decoded %d samples, want %d", m, octet, len(pcm), FrameSamples)
			}
		}
	}
}

func TestDecodeShortPayloadErrors(t *testing.T) {
	d := NewDecoder(DecoderConfig{OctetAligned: true})
	if _, err := d.Decode(nil); err == nil {
		t.Error("expected error on empty payload")
	}
}
