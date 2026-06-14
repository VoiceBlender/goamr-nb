package amrnb

// Encoder LSP analysis/quantization wrapper, ported from opencore-amrnb lsp.cpp
// (lsp). Per frame it converts the analysis A(z) to LSPs (azLsp), quantizes them
// (qPlsf3/qPlsf5), and interpolates both the unquantized LP filters (az, for the
// perceptual weighting) and the quantized ones (azQ, for synthesis) across the
// four subframes, returning the LSF codebook indices. Bit-exact with the
// reference.

// lspEncState holds the encoder's LSP predictor memories.
type lspEncState struct {
	lspOld  [cM]int16
	lspOldQ [cM]int16
	q       qPlsfState
}

func (st *lspEncState) reset() {
	copy(st.lspOld[:], lsp_init_data[:cM])
	copy(st.lspOldQ[:], lsp_init_data[:cM])
	st.q.reset()
}

// lsp processes one frame's LP analysis. az holds the unquantized A(z) with the
// per-window filters already placed (A_mid at [MP1] for MR122, A_new at [3*MP1]);
// on return az and azQ hold the interpolated unquantized/quantized filters for
// all four subframes. The LSF indices are written to anap; the count is
// returned. Mirrors lsp.
func (st *lspEncState) lsp(mode int, az, azQ, anap []int16) int {
	var lspNew [cM]int16
	if mode == dMR122 {
		var lspMid, lspMidQ, lspNewQ [cM]int16
		azLsp(az[cMP1:], lspMid[:], st.lspOld[:])
		azLsp(az[3*cMP1:], lspNew[:], lspMid[:])
		intLpc1and3_2(st.lspOld[:], lspMid[:], lspNew[:], az)

		idx := st.q.qPlsf5(lspMid[:], lspNew[:], lspMidQ[:], lspNewQ[:])
		intLpc1and3(st.lspOldQ[:], lspMidQ[:], lspNewQ[:], azQ)
		copy(anap[:5], idx[:])
		copy(st.lspOldQ[:], lspNewQ[:])
		copy(st.lspOld[:], lspNew[:])
		return 5
	}

	var lspNewQ [cM]int16
	azLsp(az[3*cMP1:], lspNew[:], st.lspOld[:])
	intLpc1to3_2(st.lspOld[:], lspNew[:], az)

	idx := st.q.qPlsf3(mode, lspNew[:], lspNewQ[:])
	intLpc1to3(st.lspOldQ[:], lspNewQ[:], azQ)
	copy(anap[:3], idx[:])
	copy(st.lspOldQ[:], lspNewQ[:])
	copy(st.lspOld[:], lspNew[:])
	return 3
}
