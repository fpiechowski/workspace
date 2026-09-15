package tui

type layoutMode int

const (
	layoutTiny layoutMode = iota
	layoutNarrow
	layoutShort
	layoutWide
)

func layoutFor(width, height int) layoutMode {
	if width < 40 || height < 12 {
		return layoutTiny
	}
	if width >= 100 && height >= 24 {
		return layoutWide
	}
	if width >= 80 && height < 24 {
		return layoutShort
	}
	return layoutNarrow
}
