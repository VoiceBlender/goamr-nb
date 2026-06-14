package amrnb

// Adaptive gain control for the post-filter, ported from opencore-amrnb agc.cpp.
// agc scales the post-filtered signal so its energy tracks the synthesis
// energy, with a per-sample gain smoothed by agc_fac across the subframe. The
// running gain is carried in agcState. Bit-exact with the reference.

const cAGC_FAC = 29491 // 0.9, the post-filter AGC factor

// agcState holds the smoothed per-sample gain across subframes.
type agcState struct {
	pastGain int16
}

// energyOld accumulates the energy of a prescaled signal (used when energyNew
// would overflow). Mirrors energy_old.
func energyOld(in []int16, lTrm int) int32 {
	var s int32
	for i := 0; i < lTrm; i++ {
		temp := in[i] >> 2
		s = L_mac(s, temp, temp)
	}
	return s
}

// energyNew returns the energy of a signal (>>4 scaled), falling back to
// energyOld on saturation. Mirrors energy_new.
func energyNew(in []int16, lTrm int) int32 {
	var s int32
	for i := 0; i < lTrm; i++ {
		s = L_mac(s, in[i], in[i])
	}
	if s != maxInt32 {
		return s >> 4
	}
	return energyOld(in, lTrm)
}

// agc scales sigOut in place so its energy approaches sigIn's, applying a gain
// smoothed across the subframe by agcFac and carried in the state. Mirrors agc.
func (st *agcState) agc(sigIn, sigOut []int16, agcFac int16, lTrm int) {
	s := energyNew(sigOut, lTrm)
	if s == 0 {
		st.pastGain = 0
		return
	}
	exp := norm_l(s) - 1
	gainOut := round_(L_shl(s, exp))

	s = energyNew(sigIn, lTrm)
	var g0 int16
	if s == 0 {
		g0 = 0
	} else {
		i := norm_l(s)
		gainIn := round_(s << i)
		exp -= i

		temp := div_s(gainOut, gainIn)
		ss := int32(temp) << 7
		ss = L_shr(ss, exp)
		ss = invSqrt(ss)
		lTemp := ss << 9
		ii := int16((lTemp + 0x8000) >> 16)
		temp = 32767 - agcFac
		g0 = int16((int32(ii) * int32(temp)) >> 15)
	}

	gain := st.pastGain
	for i := 0; i < lTrm; i++ {
		gain = int16((int32(gain) * int32(agcFac)) >> 15)
		gain += g0
		lTemp := (int32(sigOut[i]) * int32(gain)) << 1
		sigOut[i] = int16(lTemp >> 13)
	}
	st.pastGain = gain
}
