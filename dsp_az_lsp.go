package amrnb

// A(z) -> LSP conversion, ported from opencore-amrnb az_lsp.cpp (Chebps, Az_lsp).
// Az_lsp finds the line spectral pairs of an order-10 LP filter by locating the
// roots of the symmetric/antisymmetric polynomials F1(z)/F2(z) on the unit
// circle, evaluated via the Chebyshev recursion (Chebps) over the cosine grid.
// This is the encoder-side inverse of lspAz. Bit-exact with the reference.

const cNC = cM / 2 // 5

// chebps evaluates the Chebyshev series of f at x. Mirrors Chebps.
func chebps(x int16, f []int16, n int) int16 {
	pf := 1
	lTemp := int32(0x01000000)
	t0 := (int32(x) << 10) + (int32(f[pf]) << 14)
	pf++
	b1h := int16(t0 >> 16)
	b1l := int16((t0 >> 1) - (int32(b1h) << 15))

	for i := 2; i < n; i++ {
		t0 = int32(b1h) * int32(x)
		t0 += (int32(b1l) * int32(x)) >> 15
		t0 <<= 2
		t0 -= lTemp
		t0 += int32(f[pf]) << 14
		pf++
		lTemp = (int32(b1h) << 16) + (int32(b1l) << 1)
		b1h = int16(t0 >> 16)
		b1l = int16((t0 >> 1) - (int32(b1h) << 15))
	}

	t0 = int32(b1h) * int32(x)
	t0 += (int32(b1l) * int32(x)) >> 15
	t0 <<= 1
	t0 -= lTemp
	t0 += int32(f[pf]) << 13

	if uint32(t0+33554432) < 67108863 {
		return int16(t0 >> 10)
	} else if t0 > 0x01ffffff {
		return maxInt16
	}
	return minInt16
}

// azLsp converts order-10 LP coefficients a[0:M+1] to line spectral pairs
// lsp[0:M]. oldLsp is reused if fewer than M roots are found. Mirrors Az_lsp.
func azLsp(a, lsp, oldLsp []int16) {
	var f1, f2 [cNC + 1]int16
	f1[0] = 1024
	f2[0] = 1024
	for i := 0; i < cNC; i++ {
		lt1 := int32(a[i+1])
		lt2 := int32(a[cM-i])
		x := int16((lt1 + lt2) >> 2)
		y := int16((lt1 - lt2) >> 2)
		x -= f1[i]
		f1[i+1] = x
		y += f2[i]
		f2[i+1] = y
	}

	nf := 0
	ip := 0
	coef := f1[:]

	xlow := grid[0]
	ylow := chebps(xlow, coef, cNC)

	j := 0
	for nf < cM && j < 60 {
		j++
		xhigh := xlow
		yhigh := ylow
		xlow = grid[j]
		ylow = chebps(xlow, coef, cNC)

		if int32(ylow)*int32(yhigh) <= 0 {
			for i := 4; i != 0; i-- {
				xmid := (xlow >> 1) + (xhigh >> 1)
				ymid := chebps(xmid, coef, cNC)
				if int32(ylow)*int32(ymid) <= 0 {
					yhigh = ymid
					xhigh = xmid
				} else {
					ylow = ymid
					xlow = xmid
				}
			}

			x := xhigh - xlow
			yv := yhigh - ylow
			var xint int16
			if yv == 0 {
				xint = xlow
			} else {
				sign := yv
				yv = abs_s(yv)
				exp := norm_s(yv)
				yv = shl(yv, exp)
				yv = div_s(16383, yv)
				yv = int16((int32(x) * int32(yv)) >> uint(19-exp))
				if sign < 0 {
					yv = -yv
				}
				xint = int16(int32(xlow) - ((int32(ylow) * int32(yv)) >> 10))
			}

			lsp[nf] = xint
			xlow = xint
			nf++

			if ip == 0 {
				ip = 1
				coef = f2[:]
			} else {
				ip = 0
				coef = f1[:]
			}
			ylow = chebps(xlow, coef, cNC)
		}
	}

	if nf < cM {
		copy(lsp[:cM], oldLsp[:cM])
	}
}
