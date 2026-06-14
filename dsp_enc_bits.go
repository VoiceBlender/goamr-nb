package amrnb

// Encoder-side bit serialization, ported from opencore-amrnb prm2bits.cpp
// (Int2bin/Prm2bits) and the ETS->IETF reordering in ets_to_wmf.cpp
// (ets_to_ietf). The chain is: parameter array -> ETS serial bits (each
// parameter expanded MSB-first to its bitno width) -> IETF storage bytes (the
// ETS bits permuted by reorderBits and packed MSB-first). The leading ToC byte
// is added by the payload layer, not here.

// int2bin writes the low nBits of value into bits[0:nBits], MSB first
// (bits[0] is the most significant bit). Mirrors Int2bin.
func int2bin(value int16, nBits int, bits []int16) {
	for i := nBits - 1; i >= 0; i-- {
		bits[i] = value & 1
		value >>= 1
	}
}

// prm2bits expands the encoder parameter array prm into the ETS serial bit
// array for the given speech mode (length modeBits[mode]). Mirrors Prm2bits.
func prm2bits(mode Mode, prm []int16) []int16 {
	bn := bitno[mode]
	ets := make([]int16, modeBits[mode])
	pos := 0
	for i, w := range bn {
		int2bin(prm[i], int(w), ets[pos:])
		pos += int(w)
	}
	return ets
}

// etsToIetf permutes the ETS serial bits into IETF storage order via
// reorderBits[mode] and packs them MSB-first into bytes. Mirrors the speech
// branch of ets_to_ietf (without the ToC byte).
func etsToIetf(mode Mode, ets []int16) []byte {
	nb := modeBits[mode]
	ro := reorderBits[mode]
	out := make([]byte, (nb+7)/8)
	for k := 0; k < nb; k++ {
		bit := ets[ro[k]] & 1
		out[k>>3] |= byte(bit) << uint(7-(k&7))
	}
	return out
}

// packFrame serializes a mode's encoder parameters into the RFC 4867 speech
// payload bytes (IETF storage order), the inverse of mimeUnsort.
func packFrame(prm []int16, mode Mode) []byte {
	return etsToIetf(mode, prm2bits(mode, prm))
}
