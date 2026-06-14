package amrnb

// LSP/LSF conversions and interpolation, ported function-by-function from the
// opencore-amrnb common sources (lsp_az.cpp, lsp_lsf.cpp, int_lpc.cpp). These
// turn the decoded line-spectral representation into the per-subframe LP
// synthesis coefficients A(z) (order 10). Bit-exact with the reference.

// getLspPol finds the coefficients of F1(z) or F2(z) from the (cosine-domain)
// LSPs by expanding the product polynomial. f must have length 6. Mirrors
// Get_lsp_pol (lsp_az.cpp).
func getLspPol(lsp []int16, f []int32) {
	fi := 0
	li := 0
	f[fi] = 0x01000000 // f[0] = 1.0
	fi++
	f[fi] = (-int32(lsp[li])) << 10 // f[1] = -2.0 * lsp[0]
	fi++
	li++
	li++ // advance lsp pointer

	for i := int16(2); i <= 5; i++ {
		f[fi] = f[fi-2]
		for j := int16(1); j < i; j++ {
			hi := int16(f[fi-1] >> 16)
			lo := int16((f[fi-1] >> 1) - (int32(hi) << 15))
			t0 := int32(hi) * int32(lsp[li])
			t0 += (int32(lo) * int32(lsp[li])) >> 15
			f[fi] += f[fi-2]
			f[fi] -= t0 << 2
			fi--
		}
		f[fi] -= int32(lsp[li]) << 10
		li++
		fi += int(i)
		li++
	}
}

// lspAz converts line spectral pairs to order-10 LP coefficients a[0:11].
// Mirrors Lsp_Az (lsp_az.cpp).
func lspAz(lsp []int16, a []int16) {
	var f1, f2 [6]int32
	getLspPol(lsp[0:], f1[:])
	getLspPol(lsp[1:], f2[:])
	for i := 5; i > 0; i-- {
		f1[i] += f1[i-1]
		f2[i] -= f2[i-1]
	}
	a[0] = 4096
	for i, j := 1, 10; i <= 5; i, j = i+1, j-1 {
		t0 := f1[i] + f2[i]
		t1 := f1[i] - f2[i]
		t0 += 1 << 12
		t1 += 1 << 12
		a[i] = int16(t0 >> 13)
		a[j] = int16(t1 >> 13)
	}
}

// lsfLsp converts normalized LSFs (Q15, 0..0.5) to LSPs (cosine domain) by
// table lookup and interpolation. Mirrors Lsf_lsp (lsp_lsf.cpp).
func lsfLsp(lsf, lsp []int16, m int16) {
	for i := int16(0); i < m; i++ {
		ind := lsf[i] >> 8        // b8-b15
		offset := lsf[i] & 0x00ff // b0-b7
		L_tmp := (int32(lspLsfTable[ind+1]) - int32(lspLsfTable[ind])) * int32(offset) >> 8
		lsp[i] = lspLsfTable[ind] + int16(L_tmp)
	}
}

// lspLsf converts LSPs (cosine domain) to normalized LSFs (Q15) via the acos
// table and per-segment slope. Mirrors Lsp_lsf (lsp_lsf.cpp).
func lspLsf(lsp, lsf []int16, m int16) {
	ind := int16(63)
	for i := m - 1; i >= 0; i-- {
		temp := lsp[i]
		for lspLsfTable[ind] < temp {
			ind--
		}
		L_tmp := (int32(temp) - int32(lspLsfTable[ind])) * int32(lspLsfSlope[ind])
		L_tmp = (L_tmp + 0x800) >> 12
		lsf[i] = int16(L_tmp) + (ind << 8)
	}
}

// intLpc1and3 interpolates LSPs across the four subframes (old/mid/new LSP
// sets) and produces A(z) for each, into Az (length 4*cMP1). Used by modes that
// transmit a mid-frame LSP set. Mirrors Int_lpc_1and3 (int_lpc.cpp).
func intLpc1and3(lspOld, lspMid, lspNew []int16, Az []int16) {
	var lsp [cM]int16
	for i := 0; i < cM; i++ {
		lsp[i] = (lspOld[i] >> 1) + (lspMid[i] >> 1)
	}
	lspAz(lsp[:], Az[0:])    // subframe 1
	lspAz(lspMid, Az[cMP1:]) // subframe 2
	for i := 0; i < cM; i++ {
		lsp[i] = (lspMid[i] >> 1) + (lspNew[i] >> 1)
	}
	lspAz(lsp[:], Az[2*cMP1:]) // subframe 3
	lspAz(lspNew, Az[3*cMP1:]) // subframe 4
}

// intLpc1to3 interpolates between only the past-frame and present-frame LSP
// sets (no mid set) across the four subframes. Mirrors Int_lpc_1to3.
func intLpc1to3(lspOld, lspNew []int16, Az []int16) {
	var lsp [cM]int16
	for i := 0; i < cM; i++ {
		temp := lspOld[i] - (lspOld[i] >> 2)
		lsp[i] = temp + (lspNew[i] >> 2)
	}
	lspAz(lsp[:], Az[0:]) // subframe 1
	for i := 0; i < cM; i++ {
		lsp[i] = (lspNew[i] >> 1) + (lspOld[i] >> 1)
	}
	lspAz(lsp[:], Az[cMP1:]) // subframe 2
	for i := 0; i < cM; i++ {
		temp := lspNew[i] - (lspNew[i] >> 2)
		lsp[i] = temp + (lspOld[i] >> 2)
	}
	lspAz(lsp[:], Az[2*cMP1:]) // subframe 3
	lspAz(lspNew, Az[3*cMP1:]) // subframe 4
}

// intLpc1and3_2 interpolates only subframes 1 and 3 (leaving 2 and 4 as the
// directly-computed A_mid/A_new), used for the unquantized analysis filters.
// Mirrors Int_lpc_1and3_2.
func intLpc1and3_2(lspOld, lspMid, lspNew, az []int16) {
	var lsp [cM]int16
	for i := 0; i < cM; i++ {
		lsp[i] = (lspOld[i] >> 1) + (lspMid[i] >> 1)
	}
	lspAz(lsp[:], az[0:]) // subframe 1
	for i := 0; i < cM; i++ {
		lsp[i] = (lspMid[i] >> 1) + (lspNew[i] >> 1)
	}
	lspAz(lsp[:], az[2*cMP1:]) // subframe 3
}

// intLpc1to3_2 interpolates subframes 1, 2 and 3 (leaving 4 as A_new). Mirrors
// Int_lpc_1to3_2.
func intLpc1to3_2(lspOld, lspNew, az []int16) {
	var lsp [cM]int16
	for i := 0; i < cM; i++ {
		temp := lspOld[i] - (lspOld[i] >> 2)
		lsp[i] = temp + (lspNew[i] >> 2)
	}
	lspAz(lsp[:], az[0:]) // subframe 1
	for i := 0; i < cM; i++ {
		lsp[i] = (lspNew[i] >> 1) + (lspOld[i] >> 1)
	}
	lspAz(lsp[:], az[cMP1:]) // subframe 2
	for i := 0; i < cM; i++ {
		temp := lspNew[i] - (lspNew[i] >> 2)
		lsp[i] = temp + (lspOld[i] >> 2)
	}
	lspAz(lsp[:], az[2*cMP1:]) // subframe 3
}
