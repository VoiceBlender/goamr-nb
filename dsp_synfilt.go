package amrnb

// LP synthesis and analysis filters, ported from opencore-amrnb syn_filt.cpp
// and residu.cpp (order 10). synFilt runs the all-pole synthesis 1/A(z) over a
// subframe, carrying M samples of filter memory; residu runs the all-zero
// analysis A(z). Both accumulate products in wrapping int32 (the reference's
// non-saturating MAC/MSU), so a straightforward per-output loop is bit-exact;
// synFilt's output clamp reproduces the reference's exact boundary behaviour.

// synClamp reproduces the Syn_filt overflow clamp: in-range values are an
// arithmetic >>12, out-of-range saturate to ±MAX_16. The asymmetric boundary
// (s == 0x07ffffff -> MIN_16) is a reference quirk preserved deliberately.
func synClamp(s int32) int16 {
	if uint32(s+134217728) < 0x0fffffff {
		return int16(s >> 12)
	} else if s > 0x07ffffff {
		return maxInt16
	}
	return minInt16
}

// synFilt computes y = x convolved with 1/A(z) over lg samples, where a[0:M+1]
// are the LP coefficients (a[0]=4096, Q12). mem[0:M] holds the M past output
// samples; if update is true it is refreshed with the last M outputs. Mirrors
// Syn_filt.
func synFilt(a, x, y []int16, lg int, mem []int16, update bool) {
	// The ten past outputs live in locals (y1 the latest), so no history
	// buffer is copied or zeroed per call. x may alias y: x[i] is read
	// before y[i] is written. mem may alias y too; it is read first.
	af := a[:cMP1]
	a0, a1, a2, a3, a4, a5 := int32(af[0]), int32(af[1]), int32(af[2]), int32(af[3]), int32(af[4]), int32(af[5])
	a6, a7, a8, a9, a10 := int32(af[6]), int32(af[7]), int32(af[8]), int32(af[9]), int32(af[10])
	m := mem[:cM]
	y1, y2, y3, y4, y5 := int32(m[9]), int32(m[8]), int32(m[7]), int32(m[6]), int32(m[5])
	y6, y7, y8, y9, y10 := int32(m[4]), int32(m[3]), int32(m[2]), int32(m[1]), int32(m[0])
	xf, yf := x[:lg], y[:lg]
	for i := range xf {
		s := 0x800 + a0*int32(xf[i]) - a1*y1 - a2*y2 - a3*y3 - a4*y4 - a5*y5 -
			a6*y6 - a7*y7 - a8*y8 - a9*y9 - a10*y10
		out := synClamp(s)
		yf[i] = out
		y10, y9, y8, y7, y6 = y9, y8, y7, y6, y5
		y5, y4, y3, y2, y1 = y4, y3, y2, y1, int32(out)
	}
	if update {
		copy(mem[:cM], y[lg-cM:lg])
	}
}

// residu computes the LP residual A(z)·input over inputLen samples. buf holds
// the input with at least M samples of history before start (buf[start-M:start]
// are the past samples). Output goes to residual[0:inputLen]. Output is
// truncated (not saturated) to 16 bits, matching the reference. Mirrors Residu.
func residu(coef, buf []int16, start, inputLen int, residual []int16) {
	// res[i] = (0x800 + sum_{j=0..M} coef[j]·buf[start+i-j]) >> 12. Reversing the
	// coefficients turns the inner sum into a fixed-window FIR over buf[start-M:]
	// (res[i] = sum_k arev[k]·buf[start-M+i+k]), so it vectorises via firRaw. The
	// reference accumulates in plain wrapping int32, so the lane-parallel order is
	// bit-identical, and adding the 0x800 rounding bias after the sum is exact
	// (integer addition is associative mod 2^32).
	var arev [cMP1]int16
	for k := 0; k <= cM; k++ {
		arev[k] = coef[cM-k]
	}
	var dstArr [cL_SUBFR]int32
	var dst []int32
	if inputLen <= cL_SUBFR {
		dst = dstArr[:inputLen]
	} else {
		dst = make([]int32, inputLen)
	}
	firRaw(dst, buf[start-cM:], arev[:])
	for i := 0; i < inputLen; i++ {
		residual[i] = int16((0x800 + dst[i]) >> 12)
	}
}
