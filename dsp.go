package amrnb

// This file holds the AMR-NB signal-processing state and the entry points the
// public Encoder/Decoder call per 20 ms frame. The fixed-point DSP itself
// (LP analysis/LSF quantization, open/closed-loop pitch, algebraic codebook
// search, gain quantization, synthesis and post-filtering, DTX) is a large
// bit-exact port of 3GPP TS 26.090 — see opencore-amrnb (Apache-2.0), which
// ships both encoder and decoder.
//
// The structs below define where that ported state lives. The encodeFrame /
// decodeFrame methods are the single integration seam where the per-block DSP
// functions are orchestrated per 20 ms frame.
//
// NOTE: the DSP is currently STUBBED — coder()/decoder() are not yet ported, so
// encodeFrame emits a zero-filled bitstream of the correct size and decodeFrame
// emits silence. The public API, RFC 4867 framing, fixed-point operators and
// SIMD kernels are complete and exercised by the pure-Go tests; the bit-exact
// DSP lands in a later phase (see the plan).

// encoderState holds the persistent encoder analysis state across frames.
type encoderState struct {
	started bool
	st      coderState
}

func (s *encoderState) reset() {
	s.started = true
	s.st.reset()
}

// encodeFrame analyses one 160-sample frame in the given mode and returns the
// RFC 4867 frame payload bytes (the sorted/packed speech bits, without the
// CMR/ToC header which the caller adds). Mirrors Speech_Encode_Frame: mask the 3
// LSBs (13-bit input), high-pass + downscale (Pre_Process), run the analysis
// (cod_amr), then serialize (prm2bits + IETF reorder via packFrame).
func (s *encoderState) encodeFrame(mode Mode, samples []int16) ([]byte, error) {
	if !s.started {
		s.reset()
	}
	in := make([]int16, cL_FRAME)
	for i := 0; i < cL_FRAME && i < len(samples); i++ {
		in[i] = samples[i] &^ 7 // delete 3 LSBs (13-bit input)
	}
	s.st.pre.process(in, cL_FRAME)

	prm, err := s.st.codAmr(int(mode), in)
	if err != nil {
		return nil, err
	}
	return packFrame(prm, mode), nil
}

// decoderState holds the persistent decoder synthesis state across frames.
type decoderState struct {
	dsp      decoderStateDSP
	lastMode Mode
	started  bool
}

func (s *decoderState) reset() {
	s.dsp.reset()
	s.lastMode = Mode1220
	s.started = true
}

// decodeFrame decodes one parsed AMR-NB frame into 160 PCM samples. Good speech
// frames are fully decoded; bad/SID/no-data frames currently emit silence
// (error concealment and CNG are not yet ported).
func (s *decoderState) decodeFrame(f frame) ([]int16, error) {
	if !s.started {
		s.reset()
	}
	synth := make([]int16, cAMR_NB_PCM_FRAME)
	if f.ft <= FTMode1220 { // speech
		mode := Mode(f.ft)
		s.lastMode = mode
		if f.q {
			params := mimeUnsort(f.data, mode)
			s.dsp.decodeSpeechFrame(int(mode), params, synth)
		}
	}
	return synth, nil
}
