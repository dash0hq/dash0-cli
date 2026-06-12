package otlp

import (
	"math"
	"testing"
	"unicode/utf8"
)

func TestSparkline_Empty(t *testing.T) {
	if got := Sparkline(nil); got != "" {
		t.Errorf("Sparkline(nil) = %q; want empty", got)
	}
	if got := Sparkline([]float64{}); got != "" {
		t.Errorf("Sparkline([]) = %q; want empty", got)
	}
}

func TestSparkline_LengthMatchesInput(t *testing.T) {
	cases := []int{1, 3, 7, 30}
	for _, n := range cases {
		series := make([]float64, n)
		for i := range series {
			series[i] = float64(i)
		}
		got := Sparkline(series)
		if cnt := utf8.RuneCountInString(got); cnt != n {
			t.Errorf("Sparkline(len %d) returned %d runes; want %d", n, cnt, n)
		}
	}
}

func TestSparkline_AllZeros(t *testing.T) {
	got := Sparkline([]float64{0, 0, 0, 0})
	if utf8.RuneCountInString(got) != 4 {
		t.Errorf("expected 4 runes; got %d", utf8.RuneCountInString(got))
	}
	for _, r := range got {
		if r != '▁' {
			t.Errorf("all-zeros series should render all lowest glyph; got %c", r)
			break
		}
	}
}

func TestSparkline_SinglePeakMaxesGlyph(t *testing.T) {
	got := Sparkline([]float64{0, 0, 5, 0, 0})
	runes := []rune(got)
	if len(runes) != 5 {
		t.Fatalf("expected 5 runes; got %d", len(runes))
	}
	if runes[2] != '▇' {
		t.Errorf("peak should be highest glyph ▇ (the cap); got %c", runes[2])
	}
	for i, r := range runes {
		if i == 2 {
			continue
		}
		if r != '▁' {
			t.Errorf("non-peak[%d] should be lowest glyph; got %c", i, r)
		}
	}
}

func TestSparkline_NegativeAndZeroFloor(t *testing.T) {
	got := Sparkline([]float64{-1, -100, 0})
	for _, r := range got {
		if r != '▁' {
			t.Errorf("negative/zero values should floor to lowest glyph; got %c", r)
		}
	}
}

func TestSparkline_NaNAndInfTreatedAsZero(t *testing.T) {
	got := Sparkline([]float64{math.NaN(), math.Inf(1), math.Inf(-1), 1})
	runes := []rune(got)
	if len(runes) != 4 {
		t.Fatalf("expected 4 runes; got %d", len(runes))
	}
	// All three pathological values should land at lowest glyph; the 1
	// becomes the new max so it ends at the highest glyph.
	for i := range 3 {
		if runes[i] != '▁' {
			t.Errorf("NaN/Inf[%d] should floor to lowest glyph; got %c", i, runes[i])
		}
	}
	if runes[3] != '▇' {
		t.Errorf("max value should hit the highest glyph; got %c", runes[3])
	}
}

func TestSparkline_NormalizedToWindowMax(t *testing.T) {
	// A small series and a large series with the same shape should render
	// the same sparkline — the ramp is normalized against the local max,
	// not an absolute scale.
	got1 := Sparkline([]float64{1, 2, 3, 4, 5})
	got2 := Sparkline([]float64{100, 200, 300, 400, 500})
	if got1 != got2 {
		t.Errorf("normalized rendering should match shape, not magnitude:\n  got1=%q\n  got2=%q", got1, got2)
	}
}

func TestSparkline_MonotonicRamp(t *testing.T) {
	// A monotonically-increasing input should render glyphs in
	// non-decreasing order along the ramp.
	got := Sparkline([]float64{1, 2, 3, 4, 5, 6, 7, 8})
	runes := []rune(got)
	for i := 1; i < len(runes); i++ {
		if runes[i] < runes[i-1] {
			t.Errorf("rendering should be monotonic; runes[%d]=%c < runes[%d]=%c",
				i, runes[i], i-1, runes[i-1])
		}
	}
}
