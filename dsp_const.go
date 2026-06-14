package amrnb

// Core AMR-NB signal-processing constants, from opencore-amrnb cnst.h and
// mode.h. AMR-NB codes 8 kHz speech in 20 ms frames of 160 samples, with an
// order-10 LP filter and four 40-sample subframes.

const (
	cL_FRAME     = 160 // frame size (samples)
	cL_FRAME_BY2 = 80  // frame size / 2
	cL_SUBFR     = 40  // subframe size
	cL_CODE      = 40  // codevector length
	cNB_SUBFR    = 4   // subframes per frame
	cL_WINDOW    = 240 // LP analysis window size
	cL_NEXT      = 40  // LP analysis look-ahead
	cL_TOTAL     = 320 // total speech analysis buffer
	cM           = 10  // LP filter order
	cMP1         = 11  // LP filter order + 1
	cL_INTERPOL  = 11  // interpolation filter length (10+1)
	cPIT_MIN     = 20  // minimum pitch lag
	cPIT_MAX     = 143 // maximum pitch lag

	cAMR_NB_PCM_FRAME = 160 // PCM samples produced/consumed per frame
	cMAX_PRM_SIZE     = 57  // max number of serial parameters (MR122)
)

// Internal coding-mode enum, matching opencore-amrnb mode.h (enum Mode). These
// are the DSP-side mode indices; the public Mode type shares the speech values
// 0..7, with dMRDTX used for SID/comfort-noise handling.
const (
	dMR475 = iota
	dMR515
	dMR59
	dMR67
	dMR74
	dMR795
	dMR102
	dMR122
	dMRDTX
	dNMODES
)

// Receive frame types, matching opencore-amrnb enum RXFrameType. The decoder
// driver maps RFC 4867 ToC frame types onto these.
const (
	cRX_SPEECH_GOOD = iota
	cRX_SPEECH_DEGRADED
	cRX_ONSET
	cRX_SPEECH_BAD
	cRX_SID_FIRST
	cRX_SID_UPDATE
	cRX_SID_BAD
	cRX_NO_DATA
	cRX_N_FRAMETYPES
)
