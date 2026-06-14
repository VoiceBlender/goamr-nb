package amrnb

import "testing"

// TestWeightAi checks bandwidth expansion: a_exp[0]=a[0] and each expanded tap
// matches the rounded a[i]·fac[i-1] product. With unity factors a_exp≈a.
func TestWeightAi(t *testing.T) {
	a := realAz()
	fac := make([]int16, cM)
	for i := range fac {
		fac[i] = 32767 // ~1.0
	}
	aExp := make([]int16, cMP1)
	weightAi(a, fac, aExp)
	if aExp[0] != a[0] {
		t.Errorf("aExp[0]=%d, want %d", aExp[0], a[0])
	}
	for i := 1; i <= cM; i++ {
		want := int16((int32(a[i])*int32(fac[i-1]) + 0x4000) >> 15)
		if aExp[i] != want {
			t.Errorf("aExp[%d]=%d, want %d", i, aExp[i], want)
		}
	}
	// A decreasing gamma factor must shrink the higher-order taps toward 0.
	g := int16(26214) // 0.8
	gp := int16(32767)
	for i := range fac {
		fac[i] = int16((int32(gp) * int32(g)) >> 15)
		gp = fac[i]
	}
	weightAi(a, fac, aExp)
	if abs16(aExp[cM]) > abs16(a[cM]) {
		t.Errorf("expansion did not shrink last tap: %d vs %d", aExp[cM], a[cM])
	}
}

func abs16(v int16) int16 {
	if v < 0 {
		return -v
	}
	return v
}

// TestPreemphasis checks y[n]=x[n]-g·x[n-1]: g=0 is identity, and the first
// sample uses the carried memory.
func TestPreemphasis(t *testing.T) {
	x := []int16{100, 200, 300, 400, 500}
	cp := append([]int16(nil), x...)
	var mem int16 = 0
	preemphasis(&mem, cp, 0, len(cp)) // g=0 -> identity
	for i := range x {
		if cp[i] != x[i] {
			t.Fatalf("g=0 not identity at %d: %d != %d", i, cp[i], x[i])
		}
	}
	if mem != x[len(x)-1] {
		t.Errorf("mem not updated to last input: %d != %d", mem, x[len(x)-1])
	}

	// Known coefficient: y[n] = x[n] - mult(g, x[n-1]).
	cp = append([]int16(nil), x...)
	mem = 50
	g := int16(16384) // 0.5 in Q15
	preemphasis(&mem, cp, g, len(cp))
	want := make([]int16, len(x))
	want[0] = sub(x[0], mult(g, 50))
	for i := 1; i < len(x); i++ {
		want[i] = sub(x[i], mult(g, x[i-1]))
	}
	for i := range x {
		if cp[i] != want[i] {
			t.Errorf("preemph at %d: %d, want %d", i, cp[i], want[i])
		}
	}
}

// TestPostProcessDCRejection checks the output high-pass filter removes DC: a
// constant input drives the output toward zero in steady state.
func TestPostProcessDCRejection(t *testing.T) {
	var st postProcessState
	sig := make([]int16, 400)
	for i := range sig {
		sig[i] = 4000 // DC
	}
	st.process(sig, len(sig))
	// Tail should be near zero (HP filter, sum(b)=0).
	var maxTail int16
	for i := 300; i < len(sig); i++ {
		if a := abs16(sig[i]); a > maxTail {
			maxTail = a
		}
	}
	if maxTail > 50 {
		t.Errorf("DC not rejected: tail max |%d| > 50", maxTail)
	}
}

// TestPostProcessStable checks the filter stays bounded for a bounded input and
// is deterministic.
func TestPostProcessStable(t *testing.T) {
	mk := func() []int16 {
		s := make([]int16, 200)
		for i := range s {
			s[i] = int16(8000 * sinApprox(float64(i)*0.4))
		}
		return s
	}
	var st1, st2 postProcessState
	a := mk()
	b := mk()
	st1.process(a, len(a))
	st2.process(b, len(b))
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("non-deterministic at %d", i)
		}
	}
}
