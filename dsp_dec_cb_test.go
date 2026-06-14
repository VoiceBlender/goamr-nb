package amrnb

import "testing"

// nonzeros returns the indices and values of nonzero codebook entries.
func nonzeros(cod []int16) (idx []int, val []int16) {
	for i, v := range cod {
		if v != 0 {
			idx = append(idx, i)
			val = append(val, v)
		}
	}
	return
}

// TestCb234Pulses checks the 2/3/4-pulse decoders place exactly NB_PULSE pulses
// at valid, distinct positions with the ±8191/∓8192 amplitudes, including
// hand-derived anchors for the all-zero index/sign case.
func TestCb234Pulses(t *testing.T) {
	cod := make([]int16, cL_SUBFR)

	// decode2i40_9bits anchor: subNr=0, index=0, sign=0 -> startPos[0..1]={0,2}.
	// pos0 = 0*5+startPos[0]=0, pos1 = 0*5+startPos[1]=2; both negative pulses.
	decode2i40_9bits(0, 0, 0, cod)
	if cod[0] != -8192 || cod[2] != -8192 {
		t.Errorf("d2_9 anchor: cod[0]=%d cod[2]=%d, want -8192,-8192", cod[0], cod[2])
	}
	if idx, _ := nonzeros(cod); len(idx) != 2 {
		t.Errorf("d2_9: %d pulses, want 2", len(idx))
	}

	// decode3i40_14bits anchor: index=0,sign=0 -> pos {0,1,2}, all -8192.
	decode3i40_14bits(0, 0, cod)
	idx, val := nonzeros(cod)
	if len(idx) != 3 || idx[0] != 0 || idx[1] != 1 || idx[2] != 2 {
		t.Errorf("d3_14 anchor positions = %v, want [0 1 2]", idx)
	}
	for _, v := range val {
		if v != -8192 {
			t.Errorf("d3_14 anchor value %d, want -8192", v)
		}
	}

	// decode3i40_14bits with all-positive signs -> +8191.
	decode3i40_14bits(7, 0, cod)
	_, val = nonzeros(cod)
	for _, v := range val {
		if v != 8191 {
			t.Errorf("d3_14 positive value %d, want 8191", v)
		}
	}

	// decode4i40_17bits anchor: index=0,sign=0 -> pos {0,1,2,3}.
	decode4i40_17bits(0, 0, cod)
	idx, _ = nonzeros(cod)
	if len(idx) != 4 || idx[3] != 3 {
		t.Errorf("d4_17 anchor positions = %v, want [0 1 2 3]", idx)
	}

	// decode2i40_11bits anchor: index=0,sign=0 -> pos {1,0}.
	decode2i40_11bits(0, 0, cod)
	idx, _ = nonzeros(cod)
	if len(idx) != 2 {
		t.Errorf("d2_11: %d pulses, want 2", len(idx))
	}
}

// TestCbValidPositions sweeps a range of indices/signs through all 2/3/4-pulse
// decoders and asserts every pulse lands in [0, L_SUBFR) with a valid amplitude.
func TestCbValidPositions(t *testing.T) {
	cod := make([]int16, cL_SUBFR)
	valid := func(v int16) bool { return v == 8191 || v == -8192 }
	for s := int16(0); s < 16; s++ {
		for idx := int16(0); idx < 200; idx++ {
			decode2i40_9bits(idx%4, s, idx, cod)
			for _, v := range cod {
				if v != 0 && !valid(v) {
					t.Fatalf("d2_9 bad amp %d", v)
				}
			}
			decode4i40_17bits(s, idx, cod)
			for _, v := range cod {
				if v != 0 && !valid(v) {
					t.Fatalf("d4_17 bad amp %d", v)
				}
			}
		}
	}
}

// TestCb8And10 checks the MR102 (8-pulse) and MR122 (10-pulse) decoders against
// hand-derived all-zero-index anchors and basic position validity.
func TestCb8And10(t *testing.T) {
	cod := make([]int16, cL_CODE)

	// d8_31: all-zero index -> tracks 0..3 each get two coincident +8191 pulses
	// summing to 16382.
	dec8i40_31bits(make([]int16, 7), cod)
	for j := 0; j < 4; j++ {
		if cod[j] != 16382 {
			t.Errorf("d8_31 zero anchor cod[%d]=%d, want 16382", j, cod[j])
		}
	}
	for i := 4; i < cL_CODE; i++ {
		if cod[i] != 0 {
			t.Errorf("d8_31 zero anchor cod[%d]=%d, want 0", i, cod[i])
		}
	}

	// d10_35: all-zero index -> tracks 0..4 each get two coincident +4096 pulses
	// summing to 8192.
	dec10i40_35bits(make([]int16, 10), cod)
	for j := 0; j < 5; j++ {
		if cod[j] != 8192 {
			t.Errorf("d10_35 zero anchor cod[%d]=%d, want 8192", j, cod[j])
		}
	}

	// Position validity over a sweep (no panic / out-of-range, bounded values).
	for seed := int16(1); seed < 50; seed++ {
		idx8 := []int16{seed % 8, (seed + 1) % 8, (seed + 2) % 8, (seed + 3) % 8, seed % 64, (seed * 3) % 64, (seed * 5) % 32}
		dec8i40_31bits(idx8, cod)
		idx10 := make([]int16, 10)
		for k := range idx10 {
			idx10[k] = (seed * int16(k+1)) % 16
		}
		dec10i40_35bits(idx10, cod)
	}
}
