package shared

import (
	"math"
	"regexp"
	"strconv"
	"strings"
)

// Clamp bounds value to [lo, hi].
func Clamp(value, lo, hi float64) float64 {
	return math.Min(hi, math.Max(lo, value))
}

// Round rounds half up to `decimals` places, like JavaScript's Math.round,
// so values match what the web client computes.
func Round(value float64, decimals int) float64 {
	f := math.Pow(10, float64(decimals))
	return math.Floor(value*f+0.5) / f
}

// RoundInt rounds half up to the nearest integer.
func RoundInt(value float64) int {
	return int(math.Floor(value + 0.5))
}

// FormatNumber renders a number with thousands separators and up to three
// decimals, like `toLocaleString('en-US')`: 12345.5 → "12,345.5".
func FormatNumber(value float64) string {
	s := strconv.FormatFloat(Round(math.Abs(value), 3), 'f', -1, 64)
	whole, frac, _ := strings.Cut(s, ".")
	var b strings.Builder
	if value < 0 && s != "0" {
		b.WriteByte('-')
	}
	for i, r := range whole {
		if i > 0 && (len(whole)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(r)
	}
	if frac != "" {
		b.WriteByte('.')
		b.WriteString(frac)
	}
	return b.String()
}

var clockTime = regexp.MustCompile(`^([01]\d|2[0-3]):[0-5]\d$`)

// IsClockTime reports whether value is a `HH:mm` time of day.
func IsClockTime(value string) bool {
	return clockTime.MatchString(value)
}
