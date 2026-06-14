package amrnb

// LSF dequantization, ported from opencore-amrnb d_plsf.cpp, d_plsf_3.cpp,
// d_plsf_5.cpp and reorder.cpp. These reconstruct the line-spectral
// frequencies from their transmitted codebook indices through a moving-average
// predictor (state carried across frames), enforce a minimum LSF spacing, and
// convert to the cosine (LSP) domain via lsfLsp. d_plsf_3 serves all modes
// except 12.2 kbit/s; d_plsf_5 serves 12.2 kbit/s (two LSP sets per frame).

const (
	cLSF_GAP = 205 // minimum distance between adjacent LSFs (reorder)

	cALPHA_3     = 29491 // 0.9   (d_plsf_3 bad-frame lean toward mean)
	cONE_ALPHA_3 = 3277  // 0.1
	cALPHA_5     = 31128 // ~0.95 (d_plsf_5 bad-frame)
	cONE_ALPHA_5 = 1639  // ~0.05

	cLSP_PRED_FAC_MR122 = 21299 // 0.65 Q15, MR122 LSP prediction factor

	cDICO1_SIZE   = 256
	cDICO2_SIZE   = 512
	cDICO3_SIZE   = 512
	cMR515_3_SIZE = 128
	cMR795_1_SIZE = 512
)

// dPlsfState holds the LSF predictor memory across frames. Mirrors D_plsfState.
type dPlsfState struct {
	pastRQ   [cM]int16 // past quantized prediction error, Q15
	pastLsfQ [cM]int16 // past dequantized LSFs, Q15
}

// reset clears the LSF predictor: past residual to zero, past LSFs to the
// 5-table mean. Mirrors D_plsf_reset.
func (st *dPlsfState) reset() {
	for i := 0; i < cM; i++ {
		st.pastRQ[i] = 0
	}
	copy(st.pastLsfQ[:], mean_lsf_5[:cM])
}

// reorderLsf enforces a minimum distance minDist between successive LSFs.
// Mirrors Reorder_lsf (reorder.cpp).
func reorderLsf(lsf []int16, minDist, n int16) {
	lsfMin := minDist
	for i := int16(0); i < n; i++ {
		if lsf[i] < lsfMin {
			lsf[i] = lsfMin
			lsfMin = lsfMin + minDist
		} else {
			lsfMin = lsf[i] + minDist
		}
	}
}

// dPlsf3 decodes the 3-split LSF for the given mode from three indices into
// lsp1q (length cM). bfi selects the bad-frame concealment path. Mirrors
// D_plsf_3.
func (st *dPlsfState) dPlsf3(mode int, bfi int16, indice []int16, lsp1q []int16) {
	var lsf1r, lsf1q [cM]int16

	if bfi != 0 {
		for i := 0; i < cM; i++ {
			temp := mult(st.pastLsfQ[i], cALPHA_3)
			index := mult(mean_lsf_3[i], cONE_ALPHA_3)
			lsf1q[i] = add(index, temp)
		}
		if mode != dMRDTX {
			for i := 0; i < cM; i++ {
				temp := mult(st.pastRQ[i], pred_fac_3[i])
				temp = add(mean_lsf_3[i], temp)
				st.pastRQ[i] = sub(lsf1q[i], temp)
			}
		} else {
			for i := 0; i < cM; i++ {
				temp := add(mean_lsf_3[i], st.pastRQ[i])
				st.pastRQ[i] = sub(lsf1q[i], temp)
			}
		}
	} else {
		var pCb1, pCb3 []int16
		var indexLimit1, indexLimit3 int16
		pCb2 := dico2_lsf_3
		indexLimit2 := int16((cDICO2_SIZE - 1) * 3)
		switch {
		case mode == dMR475 || mode == dMR515:
			pCb1, pCb3 = dico1_lsf_3, mr515_3_lsf
			indexLimit1, indexLimit3 = (cDICO1_SIZE-1)*3, (cMR515_3_SIZE-1)*4
		case mode == dMR795:
			pCb1, pCb3 = mr795_1_lsf, dico3_lsf_3
			indexLimit1, indexLimit3 = (cMR795_1_SIZE-1)*3, (cDICO3_SIZE-1)*4
		default: // MR59, MR67, MR74, MR102, MRDTX
			pCb1, pCb3 = dico1_lsf_3, dico3_lsf_3
			indexLimit1, indexLimit3 = (cDICO1_SIZE-1)*3, (cDICO3_SIZE-1)*4
		}

		index := indice[0]
		temp := index + (index << 1)
		if temp > indexLimit1 {
			temp = indexLimit1
		}
		lsf1r[0] = pCb1[temp]
		lsf1r[1] = pCb1[temp+1]
		lsf1r[2] = pCb1[temp+2]

		index = indice[1]
		if mode == dMR475 || mode == dMR515 {
			index <<= 1 // these modes use every second entry
		}
		temp = index + (index << 1)
		if temp > indexLimit2 {
			temp = indexLimit2
		}
		lsf1r[3] = pCb2[temp]
		lsf1r[4] = pCb2[temp+1]
		lsf1r[5] = pCb2[temp+2]

		index = indice[2]
		temp = index << 2
		if temp > indexLimit3 {
			temp = indexLimit3
		}
		lsf1r[6] = pCb3[temp]
		lsf1r[7] = pCb3[temp+1]
		lsf1r[8] = pCb3[temp+2]
		lsf1r[9] = pCb3[temp+3]

		if mode != dMRDTX {
			for i := 0; i < cM; i++ {
				temp = mult(st.pastRQ[i], pred_fac_3[i])
				temp = add(mean_lsf_3[i], temp)
				lsf1q[i] = add(lsf1r[i], temp)
				st.pastRQ[i] = lsf1r[i]
			}
		} else {
			for i := 0; i < cM; i++ {
				temp = add(mean_lsf_3[i], st.pastRQ[i])
				lsf1q[i] = add(lsf1r[i], temp)
				st.pastRQ[i] = lsf1r[i]
			}
		}
	}

	reorderLsf(lsf1q[:], cLSF_GAP, cM)
	copy(st.pastLsfQ[:], lsf1q[:])
	lsfLsp(lsf1q[:], lsp1q, cM)
}

// dPlsf5 decodes the 5-split LSF (MR122) from five indices into two LSP sets
// lsp1q and lsp2q (each length cM). Mirrors D_plsf_5.
func (st *dPlsfState) dPlsf5(bfi int16, indice []int16, lsp1q, lsp2q []int16) {
	var lsf1r, lsf2r, lsf1q, lsf2q [cM]int16

	if bfi != 0 {
		for i := 0; i < cM; i++ {
			temp := int16((int32(st.pastLsfQ[i]) * cALPHA_5) >> 15)
			sign := int16((int32(mean_lsf_5[i]) * cONE_ALPHA_5) >> 15)
			lsf1q[i] = add(sign, temp)
			lsf2q[i] = lsf1q[i]
			temp = int16((int32(st.pastRQ[i]) * cLSP_PRED_FAC_MR122) >> 15)
			temp = add(mean_lsf_5[i], temp)
			st.pastRQ[i] = sub(lsf2q[i], temp)
		}
	} else {
		temp := shl(indice[0], 2)
		lsf1r[0] = dico1_lsf_5[temp]
		lsf1r[1] = dico1_lsf_5[temp+1]
		lsf2r[0] = dico1_lsf_5[temp+2]
		lsf2r[1] = dico1_lsf_5[temp+3]

		temp = shl(indice[1], 2)
		lsf1r[2] = dico2_lsf_5[temp]
		lsf1r[3] = dico2_lsf_5[temp+1]
		lsf2r[2] = dico2_lsf_5[temp+2]
		lsf2r[3] = dico2_lsf_5[temp+3]

		sign := indice[2] & 1
		var ii int16
		if indice[2] < 0 {
			ii = ^(^indice[2] >> 1)
		} else {
			ii = indice[2] >> 1
		}
		temp = shl(ii, 2)
		if sign == 0 {
			lsf1r[4] = dico3_lsf_5[temp]
			lsf1r[5] = dico3_lsf_5[temp+1]
			lsf2r[4] = dico3_lsf_5[temp+2]
			lsf2r[5] = dico3_lsf_5[temp+3]
		} else {
			lsf1r[4] = negate(dico3_lsf_5[temp])
			lsf1r[5] = negate(dico3_lsf_5[temp+1])
			lsf2r[4] = negate(dico3_lsf_5[temp+2])
			lsf2r[5] = negate(dico3_lsf_5[temp+3])
		}

		temp = shl(indice[3], 2)
		lsf1r[6] = dico4_lsf_5[temp]
		lsf1r[7] = dico4_lsf_5[temp+1]
		lsf2r[6] = dico4_lsf_5[temp+2]
		lsf2r[7] = dico4_lsf_5[temp+3]

		temp = shl(indice[4], 2)
		lsf1r[8] = dico5_lsf_5[temp]
		lsf1r[9] = dico5_lsf_5[temp+1]
		lsf2r[8] = dico5_lsf_5[temp+2]
		lsf2r[9] = dico5_lsf_5[temp+3]

		for i := 0; i < cM; i++ {
			temp = mult(st.pastRQ[i], cLSP_PRED_FAC_MR122)
			temp = add(mean_lsf_5[i], temp)
			lsf1q[i] = add(lsf1r[i], temp)
			lsf2q[i] = add(lsf2r[i], temp)
			st.pastRQ[i] = lsf2r[i]
		}
	}

	reorderLsf(lsf1q[:], cLSF_GAP, cM)
	reorderLsf(lsf2q[:], cLSF_GAP, cM)
	copy(st.pastLsfQ[:], lsf2q[:])
	lsfLsp(lsf1q[:], lsp1q, cM)
	lsfLsp(lsf2q[:], lsp2q, cM)
}
