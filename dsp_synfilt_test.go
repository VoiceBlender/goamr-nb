package amrnb

import "testing"

// realAz returns a stable order-10 A(z) from the initial LSPs.
func realAz() []int16 {
	a := make([]int16, cMP1)
	lspAz(lsp_init_data, a)
	return a
}

// TestSynFiltTrivial: with A(z)=1 (a[0]=4096, rest 0) synthesis reproduces input.
func TestSynFiltTrivial(t *testing.T) {
	a := make([]int16, cMP1)
	a[0] = 4096
	x := make([]int16, cL_SUBFR)
	for i := range x {
		x[i] = int16(1000 * sinApprox(float64(i)*0.3))
	}
	y := make([]int16, cL_SUBFR)
	mem := make([]int16, cM)
	synFilt(a, x, y, cL_SUBFR, mem, false)
	for i := range x {
		if y[i] != x[i] {
			t.Fatalf("trivial synFilt at %d: %d != %d", i, y[i], x[i])
		}
	}
}

// TestResiduTrivial: with A(z)=1 the residual equals the input.
func TestResiduTrivial(t *testing.T) {
	a := make([]int16, cMP1)
	a[0] = 4096
	const N = cL_SUBFR
	buf := make([]int16, cM+N)
	for i := 0; i < N; i++ {
		buf[cM+i] = int16(1500 * sinApprox(float64(i)*0.2))
	}
	res := make([]int16, N)
	residu(a, buf, cM, N, res)
	for i := 0; i < N; i++ {
		if res[i] != buf[cM+i] {
			t.Fatalf("trivial residu at %d: %d != %d", i, res[i], buf[cM+i])
		}
	}
}

// TestAnalysisSynthesisRoundTrip: residu (analysis) then synFilt (synthesis)
// with a real A(z) reconstructs the signal to within small quantization error.
func TestAnalysisSynthesisRoundTrip(t *testing.T) {
	a := realAz()
	const N = cL_SUBFR
	buf := make([]int16, cM+N) // M zeros history + signal
	for i := 0; i < N; i++ {
		buf[cM+i] = int16(2000 * sinApprox(float64(i)*0.25))
	}
	res := make([]int16, N)
	residu(a, buf, cM, N, res)

	out := make([]int16, N)
	mem := make([]int16, cM)
	synFilt(a, res, out, N, mem, false)

	var maxd int
	for i := 0; i < N; i++ {
		d := int(out[i]) - int(buf[cM+i])
		if d < 0 {
			d = -d
		}
		if d > maxd {
			maxd = d
		}
	}
	if maxd > 8 {
		t.Errorf("round-trip max abs error %d, want <= 8", maxd)
	}
}

// TestSynFiltMemoryContinuity: synthesizing a 2N block in one call must equal
// synthesizing it as two N blocks with memory carried between them.
func TestSynFiltMemoryContinuity(t *testing.T) {
	a := realAz()
	const N = cL_SUBFR
	x := make([]int16, 2*N)
	for i := range x {
		x[i] = int16(1800 * sinApprox(float64(i)*0.15))
	}

	whole := make([]int16, 2*N)
	mem0 := make([]int16, cM)
	synFilt(a, x, whole, 2*N, mem0, false)

	split := make([]int16, 2*N)
	mem := make([]int16, cM)
	synFilt(a, x[:N], split[:N], N, mem, true)
	synFilt(a, x[N:], split[N:], N, mem, true)

	for i := 0; i < 2*N; i++ {
		if whole[i] != split[i] {
			t.Fatalf("continuity mismatch at %d: whole=%d split=%d", i, whole[i], split[i])
		}
	}
}
