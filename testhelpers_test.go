package amrnb

// Shared deterministic test helpers (no math/strconv imports, matching the
// goamr-wb convention so tests stay dependency-free and reproducible).

// sinApprox is a Taylor-series sine good enough to synthesize deterministic
// speech-like test input without importing math.
func sinApprox(x float64) float64 {
	const pi = 3.14159265358979
	for x > pi {
		x -= 2 * pi
	}
	for x < -pi {
		x += 2 * pi
	}
	x2 := x * x
	return x * (1 - x2/6*(1-x2/20*(1-x2/42)))
}

// itoa converts a non-negative int to its decimal string without strconv.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [12]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
