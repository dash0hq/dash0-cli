package pods

import (
	"math"
	"strconv"
	"strings"
)

// Numbers with their units, ported from ditt0-cli-design-system/components/foundations/units/units.js: a space between the number and an
// abbreviation (3.2 GB), 3 significant digits and 2 decimals at most, one scale for every value of an axis.

type scale struct {
	divide float64
	suffix string
}

type step struct {
	divide float64
	suffix string
	gap    string
}

var bytesSteps = []step{{1e15, "PB", " "}, {1e12, "TB", " "}, {1e9, "GB", " "}, {1e6, "MB", " "}, {1e3, "kB", " "}, {1, "B", " "}}

var timeSteps = []step{{86400, "d", " "}, {3600, "h", " "}, {60, "min", " "}, {1, "s", " "}, {1e-3, "ms", " "}, {1e-6, "μs", " "}, {1e-9, "ns", " "}}

// A count, with unit written after it ('cores', or '/s' with no space: 12.5K/s), takes K from 10,000: 1128 reads 1128,
// not 1.13K
func countSteps(unit string) []step {
	tail := ""
	if strings.HasPrefix(unit, "/") {
		tail = unit
	} else if unit != "" {
		tail = " " + unit
	}
	return []step{{1e12, "T" + tail, ""}, {1e9, "B" + tail, ""}, {1e6, "M" + tail, ""}, {1e3, "K" + tail, ""}, {1, tail, ""}}
}

func scaleFor(unit string, max float64) scale {
	abs := math.Abs(max)
	steps, from := countSteps(unit), 10.0
	switch unit {
	case "percent":
		return scale{1, "%"}
	case "bytes":
		steps, from = bytesSteps, 1
	case "seconds":
		steps, from = timeSteps, 1
	}
	for _, s := range steps {
		if abs >= s.divide*from {
			return scale{s.divide, s.gap + s.suffix}
		}
	}
	last := steps[len(steps)-1]
	return scale{last.divide, last.gap + last.suffix}
}

// 3 significant digits, 2 decimals at most, no trailing zeros: 12.5, 850, 0.85
func digits(n float64) string {
	abs := math.Abs(n)
	fixed := 2
	if abs >= 100 {
		fixed = 0
	} else if abs >= 10 {
		fixed = 1
	}
	p := math.Pow(10, float64(fixed))
	return strconv.FormatFloat(math.Round(n*p)/p, 'f', -1, 64)
}

func formatValue(v *float64, unit string) string {
	if v == nil {
		return "no data"
	}
	s := scaleFor(unit, *v)
	return digits(*v/s.divide) + s.suffix
}

func formatNumber(v float64, s scale) string { return digits(v / s.divide) }

// figures significant figures, so 0.8359 reads 0.84 and 698.51 reads 699 (timeseries.js format)
func precise(v float64, figures int) string {
	if math.Abs(v) >= math.Pow(10, float64(figures)) {
		return strconv.FormatFloat(math.Round(v), 'f', -1, 64)
	}
	f, _ := strconv.ParseFloat(strconv.FormatFloat(v, 'g', figures, 64), 64)
	return strconv.FormatFloat(f, 'f', -1, 64)
}
