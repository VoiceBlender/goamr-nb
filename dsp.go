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

// encoderState holds the encoder state across frames: cod_amr's state, the
// input high-pass and the SID scheduler. Mirrors Speech_Encode_FrameState
// plus the sid_sync state of AMREncode.
type encoderState struct {
	started bool
	dtx     bool
	st      coderState
	sid     sidSyncState
}

// reset mirrors GSMInitEncode / Speech_Encode_Frame_reset and sid_sync_reset.
// The DTX setting is kept.
func (s *encoderState) reset() {
	s.st.reset()
	s.st.dtx = s.dtx
	if s.dtx {
		s.st.vad.reset()
		s.st.dtxEnc.reset()
	}
	s.sid.reset()
	s.started = true
}

// encodeFrame analyses one 160-sample frame in the given mode and returns
// its RFC 4867 frame bytes (without the CMR/ToC header the caller adds): a
// speech frame, a SID frame, or none for NO_DATA. Its frame type is returned
// by encodeFrameInto.
func (s *encoderState) encodeFrame(mode Mode, samples []int16) ([]byte, error) {
	f, err := s.encodeFrameInto(make([]byte, maxFrameBytes), mode, samples)
	return f.data, err
}

// encodeFrameInto encodes one frame into dst (at least maxFrameBytes) and
// returns it with its frame type: the speech mode, FTSID, or FTNoData (no
// data bytes). Mirrors GSMEncodeFrame, which masks the 3 LSBs (13-bit input),
// high-passes and halves the input (Pre_Process) and runs cod_amr, followed
// by AMREncode's sid_sync and IETF packing (ets_to_ietf).
func (s *encoderState) encodeFrameInto(dst []byte, mode Mode, samples []int16) (frame, error) {
	if !s.started {
		s.reset()
	}
	var in [cL_FRAME]int16
	for i := range in {
		in[i] = samples[i] &^ 7 // delete 3 LSBs (13-bit input)
	}
	s.st.pre.process(in[:], cL_FRAME)

	prm, used := s.st.codAmr(int(mode), in[:])
	switch s.sid.frameType(used) {
	case txSpeechGood:
		n := mode.Bytes()
		packFrameInto(dst[:n], prm, mode)
		return frame{ft: int(mode), q: true, data: dst[:n]}, nil
	case txNoData:
		return frame{ft: FTNoData, q: true, data: dst[:0]}, nil
	default:
		n := (sidBits + 7) / 8
		packSIDInto(dst[:n], prm, s.sid.prevFT == txSIDUpdate, mode)
		return frame{ft: FTSID, q: true, data: dst[:n]}, nil
	}
}

// decoderState holds the decoder state across frames: Decoder_amr's state,
// the post-filter and output high-pass, and the mode of the last frame (used
// for frames without data). Mirrors Speech_Decode_FrameState.
type decoderState struct {
	dsp      decoderStateDSP
	pst      postFilterState
	post     postProcessState
	prevMode Mode
	started  bool
}

// reset mirrors Speech_Decode_Frame_reset.
func (s *decoderState) reset() {
	s.dsp.reset(false)
	s.pst.reset()
	s.post = postProcessState{}
	s.prevMode = Mode0475
	s.started = true
}

// decodeFrame decodes one parsed frame into 160 new PCM samples.
func (s *decoderState) decodeFrame(f frame) ([]int16, error) {
	out := make([]int16, cAMR_NB_PCM_FRAME)
	return out, s.decodeFrameInto(f, out)
}

// decodeFrameInto decodes one parsed frame into dst[0:160]. It maps the frame
// type and Q bit onto a receive frame type, as AMRDecode does for the MIME
// storage format, then decodes as GSMFrameDecode does: Decoder_amr, the
// post-filter, the output high-pass and the 13-bit truncation.
//
// opencore-amrnb ignores the Q bit; here a speech frame with Q=0 is decoded
// as a bad frame and a SID frame with Q=0 as a bad SID (RFC 4867, section
// 4.3.2). FT 14 (SPEECH_LOST, 3GPP TS 26.101) is a bad frame in the previous
// mode, and FT 15 (NO_DATA) a frame without data in the previous mode.
func (s *decoderState) decodeFrameInto(f frame, dst []int16) error {
	if !s.started {
		s.reset()
	}
	var prm [cMAX_PRM_SIZE]int16
	mode := s.prevMode
	var rx int
	switch {
	case f.ft <= FTMode1220:
		mode = Mode(f.ft)
		rx = cRX_SPEECH_GOOD
		if !f.q {
			rx = cRX_SPEECH_BAD
		}
		unpackParams(mode, f.data, prm[:])
	case f.ft == FTSID:
		var sti bool
		sti, mode = unpackSID(f.data, prm[:])
		switch {
		case !f.q:
			rx = cRX_SID_BAD
		case sti:
			rx = cRX_SID_UPDATE
		default:
			rx = cRX_SID_FIRST
		}
	case f.ft == FTSpeechLost:
		rx = cRX_SPEECH_BAD
	case f.ft == FTNoData:
		rx = cRX_NO_DATA
	default:
		return errBadFrameType
	}

	var at [4 * cMP1]int16
	synth := dst[:cL_FRAME]
	s.dsp.decodeAmr(int(mode), prm[:], rx, synth, at[:])
	s.pst.postFilter(int(mode), synth, at[:])
	s.post.process(synth, cL_FRAME)
	for i := range synth {
		synth[i] &^= 7 // truncate to 13 bits
	}
	s.prevMode = mode
	return nil
}
