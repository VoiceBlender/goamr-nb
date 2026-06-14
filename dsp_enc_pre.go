package amrnb

// Per-subframe target-signal and impulse-response setup, ported from
// opencore-amrnb spreproc.cpp (subframePreProc). For each subframe it builds the
// perceptually weighted LP filters A(z/γ1)/A(z/γ2), the weighted synthesis
// impulse response h1, the LP residual (exc/res2), and the target signal xn for
// the pitch search — feeding the closed-loop pitch and codebook searches. Uses
// weightAi/synFilt/residu (all already ported). Bit-exact with the reference.

// perceptual-weighting spectral expansion factors (cod_amr.cpp).
var gamma1 = [cM]int16{30802, 28954, 27217, 25584, 24049, 22606, 21250, 19975, 18777, 17650}
var gamma1_12k2 = [cM]int16{29491, 26542, 23888, 21499, 19349, 17414, 15672, 14105, 12694, 11425}
var gamma2 = [cM]int16{19661, 11797, 7078, 4247, 2548, 1529, 917, 550, 330, 198}

// subframePreProc computes, for one subframe:
//   - h1[0:L_SUBFR]   the weighted synthesis impulse response,
//   - exc/res2        the LP residual,
//   - error           the synthesis error,
//   - xn[0:L_SUBFR]   the pitch-search target.
//
// a/aq are the unquantized/quantized LP coefficients for this subframe (M+1
// each). speech is positioned so speech[speechOff-M : speechOff+L_SUBFR] is the
// current subframe with history. memErr and memW0 are the synthesis-error and
// weighting-filter memories (M each), read but not updated here. Mirrors
// subframePreProc.
func subframePreProc(mode int, a, aq, speech []int16, speechOff int, memErr, memW0 []int16, exc, h1, xn, res2, errSig []int16) {
	var ap1, ap2 [cMP1]int16
	g1 := gamma1[:]
	if mode == dMR122 || mode == dMR102 {
		g1 = gamma1_12k2[:]
	}
	weightAi(a, g1, ap1[:])
	weightAi(a, gamma2[:], ap2[:])

	// h1 = impulse response of A(z/γ1) / (Aq(z)·A(z/γ2))
	var aiZero [cL_SUBFR]int16
	copy(aiZero[:cMP1], ap1[:cMP1]) // Ap1 then zeros
	zero := make([]int16, cM)
	synFilt(aq, aiZero[:], h1, cL_SUBFR, zero, false)
	for i := range zero {
		zero[i] = 0
	}
	synFilt(ap2[:], h1, h1, cL_SUBFR, zero, false)

	// LP residual and synthesis error
	residu(aq, speech, speechOff, cL_SUBFR, res2)
	copy(exc[:cL_SUBFR], res2[:cL_SUBFR])

	// error = synthesis of the residual (with error memory)
	memErrCopy := make([]int16, cM)
	copy(memErrCopy, memErr[:cM])
	synFilt(aq, exc, errSig, cL_SUBFR, memErrCopy, false)

	// xn = weighted error (target for pitch search)
	// Residu(Ap1, error): error needs M history -> supplied as memErr before it.
	var eb [cM + cL_SUBFR]int16
	copy(eb[:cM], memErr[:cM])
	copy(eb[cM:], errSig[:cL_SUBFR])
	residu(ap1[:], eb[:], cM, cL_SUBFR, xn)

	memW0Copy := make([]int16, cM)
	copy(memW0Copy, memW0[:cM])
	synFilt(ap2[:], xn, xn, cL_SUBFR, memW0Copy, false)
}
