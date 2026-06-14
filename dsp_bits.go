package amrnb

// Decoder-side bit de-serialization, ported from opencore-amrnb bits2prm.cpp
// (Bin2int/Bits2prm) and the IETF->ETS reordering in wmf_to_ets.cpp (used for
// MIME_IETF input). The chain inverts the encoder side: IETF storage bytes ->
// ETS serial bits (unpacked MSB-first then inverse-permuted by reorderBits) ->
// parameter array (each parameter read MSB-first from its bitno width).

// bin2int reads nBits from bits[0:nBits], MSB first (bits[0] is the most
// significant bit), and returns the assembled value. Mirrors Bin2int.
func bin2int(nBits int, bits []int16) int16 {
	var v int16
	for i := 0; i < nBits; i++ {
		v = (v << 1) | (bits[i] & 1)
	}
	return v
}

// ietfToEts unpacks the RFC 4867 speech payload bytes (IETF storage order,
// MSB-first) and inverse-permutes them via reorderBits[mode] into the ETS
// serial bit array. Mirrors the speech branch of wmf_to_ets.
func ietfToEts(mode Mode, data []byte) []int16 {
	nb := modeBits[mode]
	ro := reorderBits[mode]
	ets := make([]int16, nb)
	for i := 0; i < nb; i++ {
		bit := (data[i>>3] >> uint(7-(i&7))) & 1
		ets[ro[i]] = int16(bit)
	}
	return ets
}

// bits2prm reads the ETS serial bit array back into the mode's parameter array.
// Mirrors Bits2prm.
func bits2prm(mode Mode, ets []int16) []int16 {
	bn := bitno[mode]
	prm := make([]int16, len(bn))
	pos := 0
	for i, w := range bn {
		prm[i] = bin2int(int(w), ets[pos:])
		pos += int(w)
	}
	return prm
}

// mimeUnsort decodes the RFC 4867 speech payload bytes for the given mode into
// the decoder parameter array (the inverse of packFrame).
func mimeUnsort(data []byte, mode Mode) []int16 {
	return bits2prm(mode, ietfToEts(mode, data))
}
