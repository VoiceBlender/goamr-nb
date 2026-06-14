package amrnb

// Output post-processing building blocks, ported from opencore-amrnb
// weight_a.cpp (Weight_Ai), preemph.cpp (preemphasis) and post_pro.cpp
// (Post_Process). weightAi applies gamma bandwidth expansion to an LP filter;
// preemphasis is the 1st-order y[n]=x[n]-g·x[n-1]; postProcess is the decoder's
// output high-pass biquad with the ×2 output scaling. Bit-exact with the
// reference.

// weightAi computes the bandwidth-expanded coefficients a_exp[i] = a[i]·fac[i-1]
// (with a_exp[0]=a[0]). Mirrors Weight_Ai.
func weightAi(a []int16, fac []int16, aExp []int16) {
	aExp[0] = a[0]
	for i := 1; i <= cM; i++ {
		aExp[i] = int16((int32(a[i])*int32(fac[i-1]) + 0x4000) >> 15)
	}
}

// preemphasis applies y[n] = x[n] - g·x[n-1] in place over L samples, carrying
// one sample of memory. Mirrors preemphasis (preemph.cpp).
func preemphasis(memPre *int16, signal []int16, g int16, L int) {
	temp := signal[L-1]
	for i := L - 1; i >= 1; i-- {
		signal[i] = sub(signal[i], mult(g, signal[i-1]))
	}
	signal[0] = sub(signal[0], mult(g, *memPre))
	*memPre = temp
}

// post-process output high-pass biquad coefficients (post_pro.cpp).
var postB = [3]int16{7699, -15398, 7699}
var postA = [3]int16{8192, 15836, -7667}

// postProcessState holds the biquad's double-precision input/output memory.
type postProcessState struct {
	x0, x1                 int16
	y1hi, y1lo, y2hi, y2lo int16
}

// process applies the output high-pass filter (with ×2 gain) in place over lg
// samples. The feedback memory is kept as (hi,lo) double precision. Mirrors
// Post_Process.
func (st *postProcessState) process(signal []int16, lg int) {
	a1, a2 := int32(postA[1]), int32(postA[2])
	b0, b1, b2 := int32(postB[0]), int32(postB[1]), int32(postB[2])
	for i := 0; i < lg; i++ {
		x2 := st.x1
		st.x1 = st.x0
		st.x0 = signal[i]

		L := int32(st.y1hi) * a1
		L += (int32(st.y1lo) * a1) >> 15
		L += int32(st.y2hi) * a2
		L += (int32(st.y2lo) * a2) >> 15
		L += int32(st.x0) * b0
		L += int32(st.x1) * b1
		L += int32(x2) * b2

		L = L_shl(L, 3)
		signal[i] = round_(L_shl(L, 1))

		st.y2hi = st.y1hi
		st.y2lo = st.y1lo
		st.y1hi = int16(L >> 16)
		st.y1lo = int16((L >> 1) - (int32(st.y1hi) << 15))
	}
}
