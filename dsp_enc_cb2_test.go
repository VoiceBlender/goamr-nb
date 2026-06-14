package amrnb

import "testing"

// TestCode2i40RoundTrip is the key cross-check: the encoder's 2-pulse search
// produces an index+sign that, decoded by the decoder's decode2i40_9bits,
// reconstructs the exact same innovation vector. T0>=L_CODE disables pitch
// sharpening so the encoder's raw pulses equal the decoder's output.
func TestCode2i40RoundTrip(t *testing.T) {
	h := makeH1()
	for subNr := int16(0); subNr < 4; subNr++ {
		x := make([]int16, cL_CODE)
		seed := uint32(0x300 + subNr*131)
		for i := range x {
			seed = seed*1664525 + 1013904223
			x[i] = int16(int32(seed>>20) - 2048)
		}
		hc := append([]int16(nil), h...)
		code := make([]int16, cL_CODE)
		y := make([]int16, cL_CODE)
		index, sign := code2i40_9bits(subNr, x, hc, cL_CODE, 0, code, y)

		dec := make([]int16, cL_CODE)
		decode2i40_9bits(subNr, sign, index, dec)
		for i := range code {
			if code[i] != dec[i] {
				t.Fatalf("subNr %d: enc code[%d]=%d != dec %d (idx=%d sign=%d)",
					subNr, i, code[i], dec[i], index, sign)
			}
		}

		// exactly two nonzero pulses with ±8191/∓8192 amplitudes
		nz := 0
		for _, v := range code {
			if v != 0 {
				nz++
				if v != 8191 && v != -8192 {
					t.Errorf("subNr %d: bad pulse amplitude %d", subNr, v)
				}
			}
		}
		if nz != 2 {
			t.Errorf("subNr %d: %d pulses, want 2", subNr, nz)
		}
	}
}

// TestCode2i40FilteredCode checks y equals the innovation convolved with the
// impulse response (y[n] = sum sign_k·h[n-pos_k], the build_code filter).
func TestCode2i40FilteredCode(t *testing.T) {
	h := makeH1()
	x := make([]int16, cL_CODE)
	for i := range x {
		x[i] = int16(1500 * sinApprox(float64(i)*0.3))
	}
	hc := append([]int16(nil), h...)
	code := make([]int16, cL_CODE)
	y := make([]int16, cL_CODE)
	code2i40_9bits(1, x, hc, cL_CODE, 0, code, y)

	// recompute y from the (sharpening-free) code and h and compare
	want := make([]int16, cL_CODE)
	for n := 0; n < cL_CODE; n++ {
		var s int32
		for k := 0; k <= n; k++ {
			if code[k] != 0 {
				// pulse amplitude is ±8191/∓8192; build_code uses ±32767 sign
				var sgn int16 = 32767
				if code[k] < 0 {
					sgn = -32768
				}
				s = L_mac(s, hc[n-k], sgn)
			}
		}
		want[n] = round_(s)
	}
	for i := range y {
		if y[i] != want[i] {
			t.Fatalf("filtered code mismatch at %d: %d != %d", i, y[i], want[i])
		}
	}
}

// TestCode2i40Deterministic confirms repeatability.
func TestCode2i40Deterministic(t *testing.T) {
	h := makeH1()
	x := make([]int16, cL_CODE)
	for i := range x {
		x[i] = int16(1200 * sinApprox(float64(i)*0.41))
	}
	run := func() int16 {
		hc := append([]int16(nil), h...)
		code := make([]int16, cL_CODE)
		y := make([]int16, cL_CODE)
		idx, _ := code2i40_9bits(2, x, hc, cL_CODE, 0, code, y)
		return idx
	}
	if run() != run() {
		t.Error("code2i40_9bits non-deterministic")
	}
}
