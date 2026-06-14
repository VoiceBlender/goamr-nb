package amrnb

// Encoder LSF quantization, ported from opencore-amrnb q_plsf_3.cpp (Q_plsf_3,
// Vq_subvec3, Vq_subvec4) and lsfwt.cpp (Lsf_wt). Q_plsf_3 is the split-VQ LSF
// quantizer for all modes except 12.2 kbit/s: it converts the analysis LSPs to
// weighted LSFs, removes the MA prediction, and searches the LSF codebooks for
// the indices the bitstream carries. It is the exact encoder counterpart of the
// decoder's dPlsf3 — feeding its indices back through dPlsf3 reproduces the same
// quantized LSPs. Bit-exact with the reference.

// qPlsfState holds the encoder LSF MA-predictor memory. Mirrors Q_plsfState.
type qPlsfState struct {
	pastRQ [cM]int16
}

func (st *qPlsfState) reset() {
	for i := range st.pastRQ {
		st.pastRQ[i] = 0
	}
}

// lsfWt computes the LSF weighting factors (squared, Q13<<3). Mirrors Lsf_wt.
func lsfWt(lsf, wf []int16) {
	wf[0] = lsf[1]
	for i := 1; i <= 8; i++ {
		wf[i] = lsf[i+1] - lsf[i-1]
	}
	wf[9] = 16384 - lsf[8] // reference's pointer ends at lsf[M-2] here, not lsf[M-1]

	for i := 0; i < cM; i++ {
		wgt := wf[i]
		temp := wgt - 1843
		if temp > 0 {
			temp = int16((int32(temp) * 6242) >> 15)
			wgt = 1843 - temp
		} else {
			temp = int16((int32(wgt) * 28160) >> 15)
			wgt = 3427 - temp
		}
		wf[i] = shl(wgt, 3)
	}
}

// vqSubvec3 finds the weighted nearest codebook entry for a 3-element LSF
// residual, writing the chosen entry back into lsfR1[0:3]. useHalf searches only
// every second codebook entry. Mirrors Vq_subvec3.
func vqSubvec3(lsfR1, dico, wf1 []int16, dicoSize int, useHalf bool) int16 {
	distMin := int32(maxInt32)
	index := int16(0)
	stride := 0
	if useHalf {
		stride = 3
	}
	l0, l1, l2 := lsfR1[0], lsfR1[1], lsfR1[2]
	w0, w1, w2 := wf1[0], wf1[1], wf1[2]

	di := 0
	advance := 3 + stride
	for i := 0; i < dicoSize; i++ {
		row := dico[di : di+3] // hoist the per-dim bounds checks
		di += advance
		t := int16((int32(w0) * int32(l0-row[0])) >> 15)
		dist := int32(t) * int32(t)
		t = int16((int32(w1) * int32(l1-row[1])) >> 15)
		dist += int32(t) * int32(t)
		t = int16((int32(w2) * int32(l2-row[2])) >> 15)
		dist += int32(t) * int32(t)

		if dist < distMin {
			distMin = dist
			index = int16(i)
		}
	}

	pd := 3 * int(index)
	if useHalf {
		pd += 3 * int(index)
	}
	lsfR1[0] = dico[pd]
	lsfR1[1] = dico[pd+1]
	lsfR1[2] = dico[pd+2]
	return index
}

// vqSubvec4 finds the weighted nearest codebook entry for a 4-element LSF
// residual, writing the chosen entry back into lsfR1[0:4]. Mirrors Vq_subvec4.
func vqSubvec4(lsfR1, dico, wf1 []int16, dicoSize int) int16 {
	distMin := int32(maxInt32)
	index := int16(0)
	l0, l1, l2, l3 := lsfR1[0], lsfR1[1], lsfR1[2], lsfR1[3]
	w0, w1, w2, w3 := wf1[0], wf1[1], wf1[2], wf1[3]

	di := 0
	for i := 0; i < dicoSize; i++ {
		row := dico[di : di+4] // hoist the per-dim bounds checks
		di += 4
		t := int16((int32(w0) * int32(l0-row[0])) >> 15)
		dist := int32(t) * int32(t)
		t = int16((int32(w1) * int32(l1-row[1])) >> 15)
		dist += int32(t) * int32(t)
		t = int16((int32(w2) * int32(l2-row[2])) >> 15)
		dist += int32(t) * int32(t)
		t = int16((int32(w3) * int32(l3-row[3])) >> 15)
		dist += int32(t) * int32(t)

		if dist < distMin {
			distMin = dist
			index = int16(i)
		}
	}

	pd := int(index) << 2
	lsfR1[0] = dico[pd]
	lsfR1[1] = dico[pd+1]
	lsfR1[2] = dico[pd+2]
	lsfR1[3] = dico[pd+3]
	return index
}

// qPlsf3 quantizes the analysis LSPs for the given mode into three split-VQ
// indices and the quantized LSP vector, updating the predictor. Mirrors
// Q_plsf_3 (non-DTX path). Returns the three indices.
func (st *qPlsfState) qPlsf3(mode int, lsp1 []int16, lsp1q []int16) [3]int16 {
	var lsf1, wf1, lsfP, lsfR1, lsf1q [cM]int16

	lspLsf(lsp1, lsf1[:], cM)
	lsfWt(lsf1[:], wf1[:])

	for i := 0; i < cM; i++ {
		temp := int16((int32(st.pastRQ[i]) * int32(pred_fac_3[i])) >> 15)
		lsfP[i] = mean_lsf_3[i] + temp
		lsfR1[i] = lsf1[i] - lsfP[i]
	}

	var indice [3]int16
	switch {
	case mode == dMR475 || mode == dMR515:
		indice[0] = vqSubvec3(lsfR1[0:], dico1_lsf_3, wf1[0:], cDICO1_SIZE, false)
		indice[1] = vqSubvec3(lsfR1[3:], dico2_lsf_3, wf1[3:], cDICO2_SIZE/2, true)
		indice[2] = vqSubvec4(lsfR1[6:], mr515_3_lsf, wf1[6:], cMR515_3_SIZE)
	case mode == dMR795:
		indice[0] = vqSubvec3(lsfR1[0:], mr795_1_lsf, wf1[0:], cMR795_1_SIZE, false)
		indice[1] = vqSubvec3(lsfR1[3:], dico2_lsf_3, wf1[3:], cDICO2_SIZE, false)
		indice[2] = vqSubvec4(lsfR1[6:], dico3_lsf_3, wf1[6:], cDICO3_SIZE)
	default: // MR59, MR67, MR74, MR102
		indice[0] = vqSubvec3(lsfR1[0:], dico1_lsf_3, wf1[0:], cDICO1_SIZE, false)
		indice[1] = vqSubvec3(lsfR1[3:], dico2_lsf_3, wf1[3:], cDICO2_SIZE, false)
		indice[2] = vqSubvec4(lsfR1[6:], dico3_lsf_3, wf1[6:], cDICO3_SIZE)
	}

	for i := 0; i < cM; i++ {
		lsf1q[i] = add(lsfR1[i], lsfP[i])
		st.pastRQ[i] = lsfR1[i]
	}

	reorderLsf(lsf1q[:], cLSF_GAP, cM)
	lsfLsp(lsf1q[:], lsp1q, cM)
	return indice
}

// LSF-5 split-VQ codebook sizes (q_plsf_5_tbl.cpp).
const (
	cDICO1_5_SIZE = 128
	cDICO2_5_SIZE = 256
	cDICO3_5_SIZE = 256
	cDICO4_5_SIZE = 256
	cDICO5_5_SIZE = 64
)

// vqSubvec finds the weighted nearest 4-element codebook entry (two LSFs from
// each of the two LSF vectors), writing it back into lsfR1[0:2]/lsfR2[0:2].
// Mirrors Vq_subvec.
func vqSubvec(lsfR1, lsfR2, dico, wf1, wf2 []int16, dicoSize int) int16 {
	distMin := int32(maxInt32)
	index := int16(0)
	w10, w11 := wf1[0], wf1[1]
	w20, w21 := wf2[0], wf2[1]
	aux1 := int32(lsfR1[0]) * int32(w10)
	aux2 := int32(lsfR1[1]) * int32(w11)
	aux3 := int32(lsfR2[0]) * int32(w20)
	aux4 := int32(lsfR2[1]) * int32(w21)

	for i := 0; i < dicoSize; i++ {
		row := dico[i*4 : i*4+4] // hoist the per-dim bounds checks
		temp := int16((aux1 - int32(w10)*int32(row[0])) >> 15)
		dist := int32(temp) * int32(temp)
		if dist >= distMin {
			continue
		}
		temp = int16((aux2 - int32(w11)*int32(row[1])) >> 15)
		dist += int32(temp) * int32(temp)
		if dist >= distMin {
			continue
		}
		temp = int16((aux3 - int32(w20)*int32(row[2])) >> 15)
		dist += int32(temp) * int32(temp)
		if dist >= distMin {
			continue
		}
		temp = int16((aux4 - int32(w21)*int32(row[3])) >> 15)
		dist += int32(temp) * int32(temp)
		if dist < distMin {
			distMin = dist
			index = int16(i)
		}
	}

	pd := int(index) << 2
	lsfR1[0] = dico[pd]
	lsfR1[1] = dico[pd+1]
	lsfR2[0] = dico[pd+2]
	lsfR2[1] = dico[pd+3]
	return index
}

// vqSubvecS is vqSubvec with sign selection: each codebook entry is tried as-is
// and negated; the index LSB encodes the sign. Mirrors Vq_subvec_s.
func vqSubvecS(lsfR1, lsfR2, dico, wf1, wf2 []int16, dicoSize int) int16 {
	distMin := int32(maxInt32)
	index := int16(0)
	sign := int16(0)
	l10, l11 := lsfR1[0], lsfR1[1]
	l20, l21 := lsfR2[0], lsfR2[1]
	w10, w11 := wf1[0], wf1[1]
	w20, w21 := wf2[0], wf2[1]

	for i := 0; i < dicoSize; i++ {
		row := dico[i*4 : i*4+4] // hoist the per-dim bounds checks
		temp := row[0]
		t1 := int16((int32(w10) * int32(l10-temp)) >> 15)
		t2 := int16((int32(w10) * int32(l10+temp)) >> 15)
		dist1 := int32(t1) * int32(t1)
		dist2 := int32(t2) * int32(t2)

		temp = row[1]
		t1 = int16((int32(w11) * int32(l11-temp)) >> 15)
		t2 = int16((int32(w11) * int32(l11+temp)) >> 15)
		dist1 += int32(t1) * int32(t1)
		dist2 += int32(t2) * int32(t2)

		if dist1 >= distMin && dist2 >= distMin {
			continue
		}

		temp = row[2]
		t1 = int16((int32(w20) * int32(l20-temp)) >> 15)
		t2 = int16((int32(w20) * int32(l20+temp)) >> 15)
		dist1 += int32(t1) * int32(t1)
		dist2 += int32(t2) * int32(t2)

		temp = row[3]
		t1 = int16((int32(w21) * int32(l21-temp)) >> 15)
		t2 = int16((int32(w21) * int32(l21+temp)) >> 15)
		dist1 += int32(t1) * int32(t1)
		dist2 += int32(t2) * int32(t2)

		if dist1 < distMin {
			distMin = dist1
			index = int16(i)
			sign = 0
		}
		if dist2 < distMin {
			distMin = dist2
			index = int16(i)
			sign = 1
		}
	}

	pd := int(index) << 2
	index <<= 1
	if sign != 0 {
		lsfR1[0] = -dico[pd]
		lsfR1[1] = -dico[pd+1]
		lsfR2[0] = -dico[pd+2]
		lsfR2[1] = -dico[pd+3]
		index++
	} else {
		lsfR1[0] = dico[pd]
		lsfR1[1] = dico[pd+1]
		lsfR2[0] = dico[pd+2]
		lsfR2[1] = dico[pd+3]
	}
	return index
}

// qPlsf5 quantizes two LSP vectors (MR122) into five indices and the two
// quantized LSP vectors, updating the predictor. The exact encoder counterpart
// of dPlsf5. Mirrors Q_plsf_5.
func (st *qPlsfState) qPlsf5(lsp1, lsp2, lsp1q, lsp2q []int16) [5]int16 {
	var lsf1, lsf2, wf1, wf2, lsfP, lsfR1, lsfR2, lsf1q, lsf2q [cM]int16

	lspLsf(lsp1, lsf1[:], cM)
	lspLsf(lsp2, lsf2[:], cM)
	lsfWt(lsf1[:], wf1[:])
	lsfWt(lsf2[:], wf2[:])

	for i := 0; i < cM; i++ {
		lsfP[i] = mean_lsf_5[i] + int16((int32(st.pastRQ[i])*cLSP_PRED_FAC_MR122)>>15)
		lsfR1[i] = lsf1[i] - lsfP[i]
		lsfR2[i] = lsf2[i] - lsfP[i]
	}

	var indice [5]int16
	indice[0] = vqSubvec(lsfR1[0:], lsfR2[0:], dico1_lsf_5, wf1[0:], wf2[0:], cDICO1_5_SIZE)
	indice[1] = vqSubvec(lsfR1[2:], lsfR2[2:], dico2_lsf_5, wf1[2:], wf2[2:], cDICO2_5_SIZE)
	indice[2] = vqSubvecS(lsfR1[4:], lsfR2[4:], dico3_lsf_5, wf1[4:], wf2[4:], cDICO3_5_SIZE)
	indice[3] = vqSubvec(lsfR1[6:], lsfR2[6:], dico4_lsf_5, wf1[6:], wf2[6:], cDICO4_5_SIZE)
	indice[4] = vqSubvec(lsfR1[8:], lsfR2[8:], dico5_lsf_5, wf1[8:], wf2[8:], cDICO5_5_SIZE)

	for i := 0; i < cM; i++ {
		lsf1q[i] = lsfR1[i] + lsfP[i]
		lsf2q[i] = lsfR2[i] + lsfP[i]
		st.pastRQ[i] = lsfR2[i]
	}

	reorderLsf(lsf1q[:], cLSF_GAP, cM)
	reorderLsf(lsf2q[:], cLSF_GAP, cM)
	lsfLsp(lsf1q[:], lsp1q, cM)
	lsfLsp(lsf2q[:], lsp2q, cM)
	return indice
}
