package amrnb

// Closed-loop fractional pitch search, ported from opencore-amrnb pitch_fr.cpp
// (Pitch_fr, Norm_Corr, searchFrac, getRange) and inter_36.cpp (Interpol_3or6).
// pitchFr refines the open-loop lag: it builds the normalized correlation of the
// target with the filtered adaptive excitation over an integer search range,
// picks the best integer lag, then interpolates to a fractional lag, and encodes
// the result with encLag3/encLag6. Bit-exact with the reference.

const cL_INTER_SRCH = 4 // closed-loop pitch search interpolation half-length

// mode-dependent search parameters (pitch_fr.cpp mode_dep_parm), indexed by the
// internal speech mode 0..7.
var modeDepParm = [8]struct {
	maxFracLag, flag3, firstFrac, lastFrac  int16
	deltaIntLow, deltaIntRange, deltaFrcLow int16
	deltaFrcRange, pitMin                   int16
}{
	{84, 1, -2, 2, 5, 10, 5, 9, cPIT_MIN},      // MR475
	{84, 1, -2, 2, 5, 10, 5, 9, cPIT_MIN},      // MR515
	{84, 1, -2, 2, 3, 6, 5, 9, cPIT_MIN},       // MR59
	{84, 1, -2, 2, 3, 6, 5, 9, cPIT_MIN},       // MR67
	{84, 1, -2, 2, 3, 6, 5, 9, cPIT_MIN},       // MR74
	{84, 1, -2, 2, 3, 6, 10, 19, cPIT_MIN},     // MR795
	{84, 1, -2, 2, 3, 6, 5, 9, cPIT_MIN},       // MR102
	{94, 0, -3, 3, 3, 6, 5, 9, cPIT_MIN_MR122}, // MR122
}

// pitchFrState carries the previous subframe's integer lag across subframes.
type pitchFrState struct {
	T0prev int16
}

func (st *pitchFrState) reset() { st.T0prev = 0 }

// getRange clamps a search range [t0min, t0max] of width deltaRange around T0.
// Mirrors getRange.
func getRange(T0, deltaLow, deltaRange, pitmin, pitmax int16) (t0min, t0max int16) {
	temp := T0 - deltaLow
	if temp < pitmin {
		temp = pitmin
	}
	t0min = temp
	temp += deltaRange
	if temp > pitmax {
		temp = pitmax
		t0min = pitmax - deltaRange
	}
	t0max = temp
	return t0min, t0max
}

// normCorr computes the normalized correlation corr[t-tMin] of the target xn
// with the adaptive excitation at each integer lag t in [tMin, tMax], filtered
// by the impulse response h. exc[excOff] is the current subframe start. Mirrors
// Norm_Corr.
func normCorr(exc []int16, excOff int, xn, h []int16, Lsubfr int, tMin, tMax int16, corr []int16) {
	var excf, scaledExcf [cL_SUBFR]int16

	k := -int(tMin)
	convolve(exc[excOff+k:], h, excf[:], Lsubfr)

	var s int32
	for j := 0; j < Lsubfr; j++ {
		t := excf[j]
		scaledExcf[j] = t >> 2
		s += int32(t) * int32(t)
	}

	var sExcf []int16
	var hFac, scaling int16
	if s <= (67108864 >> 1) {
		sExcf = excf[:]
		hFac = 12
		scaling = 0
	} else {
		sExcf = scaledExcf[:]
		hFac = 14
		scaling = 2
	}

	xnf := xn[:Lsubfr]    // hoist bounds out of the inner correlation loop
	sef := sExcf[:Lsubfr] // (both length Lsubfr)
	hf := h[:Lsubfr]
	for i := tMin; i <= tMax; i++ {
		sc := firDot(xnf, sef)
		s2 := firDot(sef, sef)
		s2 <<= 1
		s2 = invSqrt(s2)
		normH := int16(s2 >> 16)
		normL := int16((s2 >> 1) - (int32(normH) << 15))
		corrH := int16(sc >> 15)
		corrL := int16(sc - (int32(corrH) << 15))
		corr[int(i)-int(tMin)] = int16(mpy_32(corrH, corrL, normH, normL))

		if i != tMax {
			k--
			temp := exc[excOff+k]
			for j := Lsubfr - 1; j >= 1; j-- {
				sef[j] = int16((int32(temp)*int32(hf[j]))>>uint(hFac)) + sef[j-1]
			}
			sef[0] = temp >> scaling
		}
	}
}

// interpol3or6 interpolates the correlation around corr[off] at fractional
// position frac (1/3 if flag3 else 1/6). Mirrors Interpol_3or6.
func interpol3or6(corr []int16, off int, frac, flag3 int16) int16 {
	if flag3 != 0 {
		frac <<= 1
	}
	if frac < 0 {
		frac += cUP_SAMP_MAX
		off--
	}
	f := int(frac)
	s := int32(0x4000)
	k := 0
	x1 := off
	x2 := off + 1
	for i := cL_INTER_SRCH >> 1; i != 0; i-- {
		s += int32(corr[x1]) * int32(inter6Pitch[f+k])
		x1--
		s += int32(corr[x2]) * int32(inter6Pitch[(cUP_SAMP_MAX-f)+k])
		x2++
		k += cUP_SAMP_MAX
		s += int32(corr[x1]) * int32(inter6Pitch[f+k])
		x1--
		s += int32(corr[x2]) * int32(inter6Pitch[(cUP_SAMP_MAX-f)+k])
		x2++
		k <<= 1
	}
	return int16(s >> 15)
}

// searchFrac refines the fractional pitch around the integer lag by maximizing
// the interpolated correlation, then normalizes the (lag, frac) pair. Mirrors
// searchFrac.
func searchFrac(lag, frac *int16, lastFrac int16, corr []int16, tMin int, flag3 int16) {
	off := int(*lag) - tMin
	max := interpol3or6(corr, off, *frac, flag3)
	for i := *frac + 1; i <= lastFrac; i++ {
		ci := interpol3or6(corr, off, i, flag3)
		if ci > max {
			max = ci
			*frac = i
		}
	}
	if flag3 == 0 {
		if *frac == -3 {
			*frac = 3
			*lag--
		}
	} else {
		if *frac == -2 {
			*frac = 1
			*lag--
		}
		if *frac == 2 {
			*frac = -1
			*lag++
		}
	}
}

// pitchFr performs the closed-loop fractional pitch search for one subframe and
// returns the integer lag T0, fractional part frac, the resolution flag resu3
// (1=1/3, 0=1/6), and the encoded analysis index. Mirrors Pitch_fr.
func (st *pitchFrState) pitchFr(mode int, Top []int16, exc []int16, excOff int, xn, h []int16, Lsubfr, iSubfr int) (T0, frac, resu3, anaIndex int16) {
	p := modeDepParm[mode]
	maxFracLag := p.maxFracLag
	flag3 := p.flag3
	frac = p.firstFrac
	lastFrac := p.lastFrac

	deltaSearch := int16(1)
	var t0min, t0max int16
	if iSubfr == 0 || iSubfr == cL_FRAME_BY2 {
		if (mode != dMR475 && mode != dMR515) || iSubfr != cL_FRAME_BY2 {
			deltaSearch = 0
			frameOffset := int16(1)
			if iSubfr == 0 {
				frameOffset = 0
			}
			t0min, t0max = getRange(Top[frameOffset], p.deltaIntLow, p.deltaIntRange, p.pitMin, cPIT_MAX)
		} else {
			t0min, t0max = getRange(st.T0prev, p.deltaFrcLow, p.deltaFrcRange, p.pitMin, cPIT_MAX)
		}
	} else {
		t0min, t0max = getRange(st.T0prev, p.deltaFrcLow, p.deltaFrcRange, p.pitMin, cPIT_MAX)
	}

	tMin := int16(int(t0min) - cL_INTER_SRCH)
	tMax := int16(int(t0max) + cL_INTER_SRCH)
	corr := make([]int16, int(tMax)-int(tMin)+1)
	normCorr(exc, excOff, xn, h, Lsubfr, tMin, tMax, corr)

	max := corr[int(t0min)-int(tMin)]
	lag := t0min
	for i := t0min + 1; i <= t0max; i++ {
		if corr[int(i)-int(tMin)] >= max {
			max = corr[int(i)-int(tMin)]
			lag = i
		}
	}

	if deltaSearch == 0 && lag > maxFracLag {
		frac = 0
	} else if deltaSearch != 0 && (mode == dMR475 || mode == dMR515 || mode == dMR59 || mode == dMR67) {
		tmpLag := st.T0prev
		if tmpLag-t0min > 5 {
			tmpLag = t0min + 5
		}
		if t0max-tmpLag > 4 {
			tmpLag = t0max - 4
		}
		switch {
		case lag == tmpLag || lag == tmpLag-1:
			searchFrac(&lag, &frac, lastFrac, corr, int(tMin), flag3)
		case lag == tmpLag-2:
			frac = 0
			searchFrac(&lag, &frac, lastFrac, corr, int(tMin), flag3)
		case lag == tmpLag+1:
			lastFrac = 0
			searchFrac(&lag, &frac, lastFrac, corr, int(tMin), flag3)
		default:
			frac = 0
		}
	} else {
		searchFrac(&lag, &frac, lastFrac, corr, int(tMin), flag3)
	}

	if flag3 != 0 {
		flag4 := int16(0)
		if mode == dMR475 || mode == dMR515 || mode == dMR59 || mode == dMR67 {
			flag4 = 1
		}
		anaIndex = encLag3(lag, frac, st.T0prev, t0min, t0max, deltaSearch, flag4)
	} else {
		anaIndex = encLag6(lag, frac, t0min, deltaSearch)
	}

	st.T0prev = lag
	resu3 = flag3
	return lag, frac, resu3, anaIndex
}
