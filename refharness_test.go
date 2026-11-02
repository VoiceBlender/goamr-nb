package amrnb

// Shared helpers for the differential tests against the stock opencore-amrnb
// library: deterministic test signals and wrappers around the reference
// harnesses in testdata/refharness (see the Makefile's harness and diff
// targets). The tests skip unless AMRNB_DIFF (decoder harness) and AMRNB_ENC
// (encoder harness) point at the built binaries.

import (
	"bytes"
	"encoding/binary"
	"math"
	"os"
	"os/exec"
	"testing"
)

// refFrame is one storage-format frame: ToC byte, then speech or SID bytes.
type refFrame []byte

func refFT(f refFrame) int { return int(f[0]>>3) & 0x0F }

func decHarness(t *testing.T) string {
	t.Helper()
	bin := os.Getenv("AMRNB_DIFF")
	if bin == "" {
		t.Skip("set AMRNB_DIFF to the opencore-amrnb reference decoder harness (make harness)")
	}
	return bin
}

func encHarness(t *testing.T) string {
	t.Helper()
	bin := os.Getenv("AMRNB_ENC")
	if bin == "" {
		t.Skip("set AMRNB_ENC to the opencore-amrnb reference encoder harness (make harness)")
	}
	return bin
}

// refEncode encodes pcm with the reference encoder, frame f in mode modeOf(f).
func refEncode(t *testing.T, pcm []int16, modeOf func(frame int) Mode, dtx bool) []refFrame {
	t.Helper()
	bin := encHarness(t)
	nf := len(pcm) / FrameSamples
	var in bytes.Buffer
	for f := 0; f < nf; f++ {
		in.WriteByte(byte(modeOf(f)))
		binary.Write(&in, binary.LittleEndian, pcm[f*FrameSamples:(f+1)*FrameSamples])
	}
	var args []string
	if dtx {
		args = append(args, "-dtx")
	}
	cmd := exec.Command(bin, args...)
	cmd.Stdin = &in
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("reference encoder: %v", err)
	}
	var frames []refFrame
	for p := 0; p < len(out); {
		nb, ok := frameBytesByFT(int(out[p]>>3) & 0x0F)
		if !ok || p+1+nb > len(out) {
			t.Fatalf("reference encoder: bad frame at byte %d", p)
		}
		frames = append(frames, refFrame(out[p:p+1+nb]))
		p += 1 + nb
	}
	if len(frames) != nf {
		t.Fatalf("reference encoder: %d frames, want %d", len(frames), nf)
	}
	return frames
}

// refDecode decodes storage frames with the reference decoder.
func refDecode(t *testing.T, frames []refFrame) []int16 {
	t.Helper()
	bin := decHarness(t)
	var in bytes.Buffer
	for _, f := range frames {
		binary.Write(&in, binary.LittleEndian, uint16(len(f)))
		in.Write(f)
	}
	cmd := exec.Command(bin)
	cmd.Stdin = &in
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("reference decoder: %v", err)
	}
	if len(out) != len(frames)*FrameSamples*2 {
		t.Fatalf("reference decoder: %d bytes, want %d", len(out), len(frames)*FrameSamples*2)
	}
	pcm := make([]int16, len(out)/2)
	binary.Read(bytes.NewReader(out), binary.LittleEndian, pcm)
	return pcm
}

func fixedMode(m Mode) func(int) Mode { return func(int) Mode { return m } }

// cycleModes switches mode every n frames, through all eight.
func cycleModes(n int) func(int) Mode {
	return func(f int) Mode { return Mode((f/n*3 + f/n/8) % 8) }
}

// lcg is the deterministic noise source of the test signals.
type lcg uint32

func (s *lcg) next() uint32 { *s = *s*1664525 + 1013904223; return uint32(*s) }

// noise returns uniform noise in [-1, 1).
func (s *lcg) noise() float64 { return float64(int32(s.next())) / (1 << 31) }

func clip16(v float64) int16 { return int16(math.Max(-32768, math.Min(32767, v))) }

// testSignal returns deterministic synthetic speech: syllables of a pulse train
// through two moving resonators, fricatives and pauses, repeated at increasing
// peak levels from very quiet to driven into clipping.
func testSignal() []int16 {
	const sr = SampleRate
	seed := lcg(1)
	var sig []float64
	type res struct{ y1, y2 float64 }
	resonate := func(r *res, x, f, bw float64) float64 {
		a := math.Exp(-math.Pi * bw / sr)
		b1 := 2 * a * math.Cos(2*math.Pi*f/sr)
		y := x + b1*r.y1 - a*a*r.y2
		r.y2, r.y1 = r.y1, y
		return y
	}
	for syl := 0; syl < 12; syl++ {
		n := int(0.18 * sr)
		f0 := 110.0 + 15*float64(syl%5)
		f1, f2 := 500.0+60*float64(syl%4), 1500.0+200*float64(syl%3)
		var r1, r2 res
		phase := 0.0
		for i := 0; i < n; i++ {
			pos := float64(i) / float64(n)
			phase += f0 * (1 + 0.2*pos) / sr
			x := 0.0
			if phase >= 1 {
				phase--
				x = 1
			}
			x += 0.02 * seed.noise()
			y := resonate(&r1, x, f1+300*pos, 90) + 0.5*resonate(&r2, x, f2-200*pos, 120)
			sig = append(sig, y*math.Sin(math.Pi*pos))
		}
		if syl%4 == 3 { // fricative
			for i := 0; i < int(0.08*sr); i++ {
				sig = append(sig, 0.3*seed.noise())
			}
		}
		if syl%6 == 5 { // pause
			sig = append(sig, make([]float64, int(0.1*sr))...)
		}
	}
	peak := 0.0
	for _, v := range sig {
		peak = math.Max(peak, math.Abs(v))
	}
	var out []int16
	for _, level := range []float64{0.003, 0.05, 0.3, 0.9, 2.0} {
		for _, v := range sig {
			out = append(out, clip16(v/peak*level*32767))
		}
	}
	return out[:len(out)/FrameSamples*FrameSamples]
}

// stressSignal returns deterministic pathological input: stationary noise at
// several levels, silence, full-scale and loud tones (tone stabilization,
// pitch-gain clipping), a square wave, an impulse train, a DC step, a sweep,
// DTMF-like dual tones and loud noise.
func stressSignal() []int16 {
	const sr = SampleRate
	seed := lcg(7)
	var out []int16
	add := func(n int, f func(i int) float64) {
		for i := 0; i < n; i++ {
			out = append(out, clip16(f(i)))
		}
	}
	sine := func(a, f float64) func(int) float64 {
		return func(i int) float64 { return a * math.Sin(2*math.Pi*f*float64(i)/sr) }
	}
	sec := int(sr)
	add(3*sec, func(int) float64 { return 1000 * seed.noise() })
	add(sec/2, func(int) float64 { return 0 })
	add(2*sec, func(int) float64 { return 100 * seed.noise() })
	add(sec, sine(32767, 1000))
	add(sec, sine(29000, 200))
	add(sec, sine(20000, 3500))
	add(sec, func(i int) float64 { // square wave, 100 Hz, full scale
		if (i/40)%2 == 0 {
			return 32767
		}
		return -32768
	})
	add(sec, func(i int) float64 { // impulse train, 125 Hz
		if i%64 == 0 {
			return 32767
		}
		return 0
	})
	add(sec/2, func(int) float64 { return 20000 })
	add(sec/2, func(int) float64 { return 0 })
	add(2*sec, func(i int) float64 { // sweep 50 Hz .. 3.5 kHz
		t := float64(i) / sr
		return 25000 * math.Sin(2*math.Pi*(50*t+(3500-50)/(2*2.0)*t*t))
	})
	for _, p := range [][2]float64{{697, 1209}, {852, 1477}, {941, 1633}} { // DTMF
		add(sec/4, func(i int) float64 { return sine(8000, p[0])(i) + sine(8000, p[1])(i) })
		add(sec/8, func(int) float64 { return 0 })
	}
	add(2*sec, func(int) float64 { return 30000 * seed.noise() })
	return out[:len(out)/FrameSamples*FrameSamples]
}

// dtxSignal returns speech bursts separated by background noise whose level
// and spectrum vary, so that the VAD switches between speech and noise and the
// comfort-noise parameters change between SID updates.
func dtxSignal() []int16 {
	const sr = SampleRate
	seed := lcg(3)
	speech := testSignal()
	var out []int16
	var lp float64
	noise := func(n int, level, color float64) {
		for i := 0; i < n; i++ {
			lp = color*lp + (1-color)*seed.noise()
			out = append(out, clip16(level*lp))
		}
	}
	burst := func(from, n int) { out = append(out, speech[from:from+n]...) }
	noise(int(1.5*sr), 300, 0)
	burst(int(0.6*sr)+len(speech)*2/5, int(1.2*sr))
	noise(int(2*sr), 600, 0.8)
	noise(int(1*sr), 60, 0.3)
	burst(len(speech)*3/5, int(1.0*sr))
	noise(int(2*sr), 2000, 0.95)
	burst(len(speech)*3/5+int(1.5*sr), int(0.3*sr))
	noise(int(3*sr), 20, 0)
	noise(int(1*sr), 0, 0)
	burst(len(speech)*4/5, int(0.8*sr))
	noise(int(2*sr), 1200, 0.5)
	return out[:len(out)/FrameSamples*FrameSamples]
}
