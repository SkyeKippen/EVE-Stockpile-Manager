package stockpile

import (
	"strconv"
	"strings"
)

// maxQuantity keeps quantities well inside what the database and ISK math
// can hold.
const maxQuantity = 1_000_000_000_000

// ParseQuantity reads a whole number like 1200, 1,200, 1.2k, 3m or 1b.
func ParseQuantity(s string) (int64, bool) {
	s = strings.ToLower(strings.ReplaceAll(strings.TrimSpace(s), ",", ""))
	multiplier := 1.0
	switch {
	case strings.HasSuffix(s, "k"):
		multiplier = 1e3
	case strings.HasSuffix(s, "m"):
		multiplier = 1e6
	case strings.HasSuffix(s, "b"):
		multiplier = 1e9
	}
	if multiplier != 1 {
		s = strings.TrimSpace(s[:len(s)-1])
	}
	n, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	n *= multiplier
	if n < 1 || n > maxQuantity || n != float64(int64(n)) {
		return 0, false
	}
	return int64(n), true
}
