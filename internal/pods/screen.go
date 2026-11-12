package pods

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

// A tiny layout layer: a block is lines of styled segments, every glyph one cell wide (box drawing, braille, block
// elements). Blocks join side by side or stacked, and get Ditt0's dim rounded border, as Ink's Box draws them.

type style struct {
	fg, bg    string
	bold, dim bool
	link      string
}

type seg struct {
	text string
	st   style
}

type line []seg
type block []line

var dimStyle = style{dim: true}

func text(s string, st style) line { return line{{s, st}} }

func (l line) width() int {
	w := 0
	for _, s := range l {
		w += utf8.RuneCountInString(s.text)
	}
	return w
}

// The line cut to w cells, so a terminal narrower than the page never wraps it onto the next row
func (l line) cut(w int) line {
	var out line
	for _, s := range l {
		r := []rune(s.text)
		if len(r) >= w {
			return append(out, seg{string(r[:w]), s.st})
		}
		out, w = append(out, s), w-len(r)
	}
	return out
}

func (b block) width() int {
	w := 0
	for _, l := range b {
		w = max(w, l.width())
	}
	return w
}

func padTo(l line, w int) line {
	if n := l.width(); n < w {
		return append(append(line{}, l...), seg{strings.Repeat(" ", w-n), style{}})
	}
	return l
}

// Blocks side by side, top aligned, gap empty cells between them
func hjoin(gap int, blocks ...block) block {
	height := 0
	for _, b := range blocks {
		height = max(height, len(b))
	}
	out := make(block, height)
	for i, b := range blocks {
		w := b.width()
		for r := 0; r < height; r++ {
			var l line
			if r < len(b) {
				l = b[r]
			}
			if i < len(blocks)-1 {
				l = padTo(padTo(l, w), w+gap)
			}
			out[r] = append(out[r], l...)
		}
	}
	return out
}

// Blocks stacked, gap empty rows between them; nil blocks are left out
func vjoin(gap int, blocks ...block) block {
	var out block
	for _, b := range blocks {
		if b == nil {
			continue
		}
		if out != nil {
			for i := 0; i < gap; i++ {
				out = append(out, line{})
			}
		}
		out = append(out, b...)
	}
	return out
}

func indent(n int, b block) block {
	out := make(block, len(b))
	for i, l := range b {
		out[i] = append(line{{strings.Repeat(" ", n), style{}}}, l...)
	}
	return out
}

// A rounded border, padX empty cells inside it either side, the content padded to width cells
func border(b block, width, padX int, edge style) block {
	inner := width - 2 - 2*padX
	pad := strings.Repeat(" ", padX)
	out := block{text("╭"+strings.Repeat("─", width-2)+"╮", edge)}
	for _, l := range b {
		row := line{{"│", edge}, {pad, style{}}}
		row = append(row, padTo(l, inner)...)
		out = append(out, append(row, seg{pad, style{}}, seg{"│", edge}))
	}
	return append(out, text("╰"+strings.Repeat("─", width-2)+"╯", edge))
}

func sgr(hex string, base int) string {
	r, _ := strconv.ParseUint(hex[1:3], 16, 8)
	g, _ := strconv.ParseUint(hex[3:5], 16, 8)
	b, _ := strconv.ParseUint(hex[5:7], 16, 8)
	return fmt.Sprintf("%d;2;%d;%d;%d", base, r, g, b)
}

func (l line) ansi() string {
	var sb strings.Builder
	for _, s := range l {
		var codes []string
		if s.st.bold {
			codes = append(codes, "1")
		}
		if s.st.dim {
			codes = append(codes, "2")
		}
		if s.st.fg != "" {
			codes = append(codes, sgr(s.st.fg, 38))
		}
		if s.st.bg != "" {
			codes = append(codes, sgr(s.st.bg, 48))
		}
		if s.st.link != "" {
			// OSC 8, so the terminal opens it on a click
			sb.WriteString("\x1b]8;;" + s.st.link + "\x1b\\")
		}
		if len(codes) > 0 {
			sb.WriteString("\x1b[" + strings.Join(codes, ";") + "m" + s.text + "\x1b[0m")
		} else {
			sb.WriteString(s.text)
		}
		if s.st.link != "" {
			sb.WriteString("\x1b]8;;\x1b\\")
		}
	}
	return sb.String()
}
