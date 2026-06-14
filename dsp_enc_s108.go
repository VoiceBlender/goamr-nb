package amrnb

// Multi-pulse algebraic codebook search, ported from opencore-amrnb s10_8pf.cpp
// (search_10and8i40). This parameterized depth-first search places 8 (MR102) or
// 10 (MR122) pulses, two tracks at a time, using a precomputed interleaved
// scratch (temp1) to cache the partial ps/alp values of the second pulse of each
// pair across the first pulse's positions. The pulse-track ordering (ipos) is
// rotated across the outer loop. The single most intricate function in the
// codec; the exact incremental criterion arithmetic is preserved (bit-exact).
func search10and8i40(nbPulse, step, nbTracks int16, dn, rr, ipos, posMax, codvec []int16) {
	const L = cL_CODE
	gsmefr := nbPulse == 10

	var index [10]int16
	var temp1 [2 * cL_CODE]int16

	i0 := posMax[ipos[0]]
	index[0] = i0

	psk := int16(-1)
	alpk := int16(1)
	for i := int16(0); i < nbPulse; i++ {
		codvec[i] = i
	}

	var i1, i2, i3, i4, i5, i6, i7 int16

	for outer := int16(1); outer < nbTracks; outer++ {
		i1 = posMax[ipos[1]]
		index[1] = i1

		ps0 := int16(int32(dn[i0]) + int32(dn[i1]))
		alp0 := int32(rr[int(i0)*L+int(i0)]) << 12
		alp0 += int32(rr[int(i1)*L+int(i1)]) << 12
		alp0 += int32(rr[int(i0)*L+int(i1)]) << 13
		alp0 += 0x8000

		// ---- pulses 2 & 3 ----
		ti := 0
		for i3v := ipos[3]; i3v < L; i3v += step {
			rrRow := rr[int(i3v)*L : int(i3v)*L+L]
			s := int32(rrRow[int(i3v)]) >> 1
			s += int32(rrRow[int(i0)])
			s += int32(rrRow[int(i1)])
			temp1[ti] = ps0 + dn[i3v]
			temp1[ti+1] = int16((s + 2) >> 2)
			ti += 2
		}

		sq := int16(-1)
		alp := int16(1)
		ps := int16(0)
		ia := ipos[2]
		ib := ipos[3]
		sbase := alp0 >> 12
		for j := ipos[2]; j < L; j += step {
			rrRow := rr[int(j)*L : int(j)*L+L]
			alp1 := (sbase + int32(rrRow[int(j)])) >> 1
			alp1 += int32(rrRow[int(i0)])
			alp1 += int32(rrRow[int(i1)])
			ps1 := dn[j]
			ti = 0
			for i3v := ipos[3]; i3v < L; i3v += step {
				ps2 := ps1 + temp1[ti]
				sq2 := int16((int32(ps2) * int32(ps2)) >> 15)
				alp2 := (alp1 + int32(rrRow[int(i3v)])) >> 2
				alp2 = (alp2 + int32(temp1[ti+1])) >> 1
				ti += 2
				if int32(sq2)*int32(alp) > int32(sq)*alp2 {
					sq, ps, alp, ia, ib = sq2, ps2, int16(alp2), j, i3v
				}
			}
		}
		i2, i3 = ia, ib
		index[2], index[3] = ia, ib

		// ---- pulses 4 & 5 ----
		alp0 = (int32(alp) << 15) + 0x8000
		ti = 0
		for i5v := ipos[5]; i5v < L; i5v += step {
			rrRow := rr[int(i5v)*L : int(i5v)*L+L]
			s := int32(rrRow[int(i5v)]) >> 1
			s += int32(rrRow[int(i0)]) + int32(rrRow[int(i1)]) + int32(rrRow[int(i2)]) + int32(rrRow[int(i3)])
			temp1[ti] = ps + dn[i5v]
			temp1[ti+1] = int16((s + 2) >> 2)
			ti += 2
		}
		sq, alp, ps, ia, ib = -1, 1, 0, ipos[4], ipos[5]
		for j := ipos[4]; j < L; j += step {
			rrRow := rr[int(j)*L : int(j)*L+L]
			alp1 := alp0 + (int32(rrRow[int(j)]) << 11)
			alp1 += int32(rrRow[int(i0)]) << 12
			alp1 += int32(rrRow[int(i1)]) << 12
			alp1 += int32(rrRow[int(i2)]) << 12
			alp1 += int32(rrRow[int(i3)]) << 12
			ps1 := dn[j]
			ti = 0
			for i5v := ipos[5]; i5v < L; i5v += step {
				ps2 := ps1 + temp1[ti]
				alp2 := alp1 + (int32(rrRow[int(i5v)]) << 12)
				alp16 := int16((alp2 + (int32(temp1[ti+1]) << 14)) >> 16)
				ti += 2
				sq2 := int16((int32(ps2) * int32(ps2)) >> 15)
				if int32(sq2)*int32(alp) > int32(sq)*int32(alp16) {
					sq, ps, alp, ia, ib = sq2, ps2, alp16, j, i5v
				}
			}
		}
		i4, i5 = ia, ib
		index[4], index[5] = ia, ib

		// ---- pulses 6 & 7 ----
		alp0 = (int32(alp) << 15) + 0x8000
		ti = 0
		for i7v := ipos[7]; i7v < L; i7v += step {
			rrRow := rr[int(i7v)*L : int(i7v)*L+L]
			s := int32(rrRow[int(i7v)]) >> 1
			s += int32(rrRow[int(i0)]) + int32(rrRow[int(i1)]) + int32(rrRow[int(i2)]) +
				int32(rrRow[int(i3)]) + int32(rrRow[int(i4)]) + int32(rrRow[int(i5)])
			temp1[ti] = ps + dn[i7v]
			temp1[ti+1] = int16((s + 4) >> 3)
			ti += 2
		}
		sq, alp, ps, ia, ib = -1, 1, 0, ipos[6], ipos[7]
		for j := ipos[6]; j < L; j += step {
			rrRow := rr[int(j)*L : int(j)*L+L]
			alp1 := alp0 + (int32(rrRow[int(j)]) << 10)
			alp1 += int32(rrRow[int(i0)]) << 11
			alp1 += int32(rrRow[int(i1)]) << 11
			alp1 += int32(rrRow[int(i2)]) << 11
			alp1 += int32(rrRow[int(i3)]) << 11
			alp1 += int32(rrRow[int(i4)]) << 11
			alp1 += int32(rrRow[int(i5)]) << 11
			ps1 := dn[j]
			ti = 0
			for i7v := ipos[7]; i7v < L; i7v += step {
				ps2 := ps1 + temp1[ti]
				alp2 := alp1 + (int32(rrRow[int(i7v)]) << 11)
				alp16 := int16((alp2 + (int32(temp1[ti+1]) << 14)) >> 16)
				ti += 2
				sq2 := int16((int32(ps2) * int32(ps2)) >> 15)
				if int32(sq2)*int32(alp) > int32(sq)*int32(alp16) {
					sq, ps, alp, ia, ib = sq2, ps2, alp16, j, i7v
				}
			}
		}
		i6, i7 = ia, ib
		index[6], index[7] = ia, ib

		// ---- pulses 8 & 9 (MR122 only) ----
		if gsmefr {
			alp0 = (int32(alp) << 15) + 0x8000
			ti = 0
			for i9v := ipos[9]; i9v < L; i9v += step {
				rrRow := rr[int(i9v)*L : int(i9v)*L+L]
				s := int32(rrRow[int(i9v)]) >> 1
				s += int32(rrRow[int(i0)]) + int32(rrRow[int(i1)]) + int32(rrRow[int(i2)]) +
					int32(rrRow[int(i3)]) + int32(rrRow[int(i4)]) + int32(rrRow[int(i5)]) +
					int32(rrRow[int(i6)]) + int32(rrRow[int(i7)])
				temp1[ti] = ps + dn[i9v]
				temp1[ti+1] = int16((s + 4) >> 3)
				ti += 2
			}
			sq, alp, ps, ia, ib = -1, 1, 0, ipos[8], ipos[9]
			for j := ipos[8]; j < L; j += step {
				rrRow := rr[int(j)*L : int(j)*L+L]
				alp1 := alp0 + (int32(rrRow[int(j)]) << 9)
				alp1 += int32(rr[int(i0)*L+int(j)]) << 10
				alp1 += int32(rr[int(i1)*L+int(j)]) << 10
				alp1 += int32(rr[int(i2)*L+int(j)]) << 10
				alp1 += int32(rr[int(i3)*L+int(j)]) << 10
				alp1 += int32(rr[int(i4)*L+int(j)]) << 10
				alp1 += int32(rr[int(i5)*L+int(j)]) << 10
				alp1 += int32(rr[int(i6)*L+int(j)]) << 10
				alp1 += int32(rr[int(i7)*L+int(j)]) << 10
				ps1 := dn[j]
				ti = 0
				for i9v := ipos[9]; i9v < L; i9v += step {
					ps2 := ps1 + temp1[ti]
					sq2 := int16((int32(ps2) * int32(ps2)) >> 15)
					alp2 := alp1 + (int32(rrRow[int(i9v)]) << 10)
					alp16 := int16((alp2 + (int32(temp1[ti+1]) << 13)) >> 16)
					ti += 2
					if int32(sq2)*int32(alp) > int32(sq)*int32(alp16) {
						sq, ps, alp, ia, ib = sq2, ps2, alp16, j, i9v
					}
				}
			}
			index[8], index[9] = ia, ib
		}

		// ---- keep the best ----
		if int32(alpk)*int32(sq) > int32(psk)*int32(alp) {
			psk = sq
			alpk = alp
			copy(codvec[:nbPulse], index[:nbPulse])
		}

		// rotate ipos[1..nbPulse-1] left by one
		pos := ipos[1]
		for j := int16(1); j < nbPulse-1; j++ {
			ipos[j] = ipos[j+1]
		}
		ipos[nbPulse-1] = pos
	}
}
