package tui

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// sanitize removes terminal control sequences before styling external text.
// It handles CSI, OSC, DCS, APC, PM, and SOS families, including BEL and ST
// terminators. Newlines and tabs remain available for viewport content.
func sanitize(value string) string {
	value = strings.ToValidUTF8(value, "�")
	runes := []rune(value)
	var out strings.Builder
	const (
		plain = iota
		escape
		intermediate
		csi
		stringControl
	)
	state := plain
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		switch state {
		case escape:
			switch r {
			case '[':
				state = csi
			case ']', 'P', '_', '^', 'X':
				state = stringControl
			case '\\':
				state = plain
			default:
				if r >= 0x20 && r <= 0x2f {
					state = intermediate
				} else {
					state = plain
				}
			}
		case intermediate:
			if r >= 0x30 && r <= 0x7e {
				state = plain
			}
		case csi:
			if r >= 0x40 && r <= 0x7e {
				state = plain
			}
		case stringControl:
			if r == '\a' || r == 0x9c {
				state = plain
			} else if r == 0x1b && i+1 < len(runes) && runes[i+1] == '\\' {
				i++
				state = plain
			}
		default:
			if r == 0x1b {
				state = escape
				continue
			}
			if r == '\n' || r == '\t' {
				out.WriteRune(r)
				continue
			}
			if r == '\r' || unicode.IsControl(r) || !utf8.ValidRune(r) {
				continue
			}
			out.WriteRune(r)
		}
	}
	return out.String()
}

func sanitizeLine(value string) string {
	value = sanitize(value)
	value = strings.ReplaceAll(value, "\n", " ")
	value = strings.ReplaceAll(value, "\t", " ")
	return strings.Join(strings.Fields(value), " ")
}
