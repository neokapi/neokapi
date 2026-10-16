package axis

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Range is one interval on a range axis, both bounds inclusive. An open upper
// bound is +Inf.
type Range struct {
	Min, Max float64
}

// ParseRange reads a range value: "13-17", an open upper bound "65-", or a
// single number "18", which is the interval holding that number alone.
func ParseRange(value string) (Range, error) {
	text := strings.TrimSpace(value)
	lo, hi, dash := strings.Cut(text, "-")
	if text == "" || lo == "" {
		return Range{}, fmt.Errorf("%q is not a range: write it as 13-17, 65- or 18", value)
	}
	minimum, err := strconv.ParseFloat(lo, 64)
	if err != nil || math.IsNaN(minimum) || math.IsInf(minimum, 0) {
		return Range{}, fmt.Errorf("%q is not a range: %q is not a number. Write it as 13-17, 65- or 18", value, lo)
	}
	if !dash {
		return Range{Min: minimum, Max: minimum}, nil
	}
	if hi == "" {
		return Range{Min: minimum, Max: math.Inf(1)}, nil
	}
	maximum, err := strconv.ParseFloat(hi, 64)
	if err != nil || math.IsNaN(maximum) || math.IsInf(maximum, 0) {
		return Range{}, fmt.Errorf("%q is not a range: %q is not a number. Write it as 13-17, 65- or 18", value, hi)
	}
	if maximum < minimum {
		return Range{}, fmt.Errorf("%q is not a range: its lower bound is above its upper bound. Write it as %s-%s", value, hi, lo)
	}
	return Range{Min: minimum, Max: maximum}, nil
}

// Contains reports whether other lies inside r.
func (r Range) Contains(other Range) bool {
	return r.Min <= other.Min && other.Max <= r.Max
}

// Overlaps reports whether the two intervals share a number.
func (r Range) Overlaps(other Range) bool {
	return r.Min <= other.Max && other.Min <= r.Max
}

// Open reports whether the range has no upper bound.
func (r Range) Open() bool { return math.IsInf(r.Max, 1) }
