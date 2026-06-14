// Package amrnb implements the AMR-NB (3GPP TS 26.071 / 26.090) narrowband
// speech codec in pure Go, with RFC 4867 RTP payload framing.
//
// AMR-NB operates on 8 kHz mono audio in 20 ms frames (160 samples). It defines
// eight speech coding modes (0..7) spanning 4.75 to 12.2 kbit/s, plus a comfort
// noise (SID) frame used by DTX.
package amrnb

// SampleRate is the fixed AMR-NB sampling rate in Hz.
const SampleRate = 8000

// FrameSamples is the number of PCM samples in one 20 ms AMR-NB frame.
const FrameSamples = 160

// Mode identifies an AMR-NB speech coding mode (0..7).
type Mode int

const (
	Mode0475 Mode = iota // 4.75 kbit/s
	Mode0515             // 5.15 kbit/s
	Mode0590             // 5.90 kbit/s
	Mode0670             // 6.70 kbit/s
	Mode0740             // 7.40 kbit/s
	Mode0795             // 7.95 kbit/s
	Mode1020             // 10.2 kbit/s
	Mode1220             // 12.2 kbit/s
	numSpeechModes
)

// Frame type (FT) values carried in the RFC 4867 ToC byte. Values 0..7 are the
// speech modes; the remainder describe comfort noise and no-data frames. Note
// that AMR-NB places SID at FT 8 (AMR-WB uses FT 9).
const (
	FTMode0475   = 0
	FTMode0515   = 1
	FTMode0590   = 2
	FTMode0670   = 3
	FTMode0740   = 4
	FTMode0795   = 5
	FTMode1020   = 6
	FTMode1220   = 7
	FTSID        = 8  // comfort noise (SID)
	FTSpeechLost = 14 // speech lost (RFC 4867)
	FTNoData     = 15 // no data
)

// modeBits is the number of speech bits per AMR-NB mode (0..7). These are the
// canonical bit counts from 3GPP TS 26.101 (and RFC 4867 Table 1a).
var modeBits = [numSpeechModes]int{
	95,  // 4.75
	103, // 5.15
	118, // 5.90
	134, // 6.70
	148, // 7.40
	159, // 7.95
	204, // 10.2
	244, // 12.2
}

// sidBits is the number of bits in an AMR-NB SID (comfort noise) frame.
const sidBits = 39

// frameBitsByFT returns the number of payload bits for a given frame type and
// whether the frame type is a recognized one that carries bits. SID and speech
// modes carry bits; SPEECH_LOST and NO_DATA carry none.
func frameBitsByFT(ft int) (bits int, ok bool) {
	switch {
	case ft >= FTMode0475 && ft <= FTMode1220:
		return modeBits[ft], true
	case ft == FTSID:
		return sidBits, true
	case ft == FTSpeechLost, ft == FTNoData:
		return 0, true
	default:
		return 0, false
	}
}

// frameBytesByFT returns the number of octet-aligned bytes occupied by a frame
// of the given type (bits rounded up to whole octets).
func frameBytesByFT(ft int) (bytes int, ok bool) {
	bits, ok := frameBitsByFT(ft)
	if !ok {
		return 0, false
	}
	return (bits + 7) / 8, true
}

// Bits returns the number of speech bits produced by this mode.
func (m Mode) Bits() int {
	if m < 0 || m >= numSpeechModes {
		return 0
	}
	return modeBits[m]
}

// Bytes returns the octet-aligned byte count of a frame in this mode.
func (m Mode) Bytes() int { return (m.Bits() + 7) / 8 }

// FrameType returns the RFC 4867 frame-type value for this mode.
func (m Mode) FrameType() int { return int(m) }

// Valid reports whether m is a defined speech mode.
func (m Mode) Valid() bool { return m >= Mode0475 && m < numSpeechModes }

// Bitrate returns the nominal bit rate of the mode in bit/s.
func (m Mode) Bitrate() int {
	switch m {
	case Mode0475:
		return 4750
	case Mode0515:
		return 5150
	case Mode0590:
		return 5900
	case Mode0670:
		return 6700
	case Mode0740:
		return 7400
	case Mode0795:
		return 7950
	case Mode1020:
		return 10200
	case Mode1220:
		return 12200
	default:
		return 0
	}
}
