package tui

// Minimum supported terminal size. Anything smaller renders the size message.
const (
	minLayoutWidth  = 40
	minLayoutHeight = 12
	// Split layouts (list plus preview) require at least this width and height.
	wideLayoutWidth  = 100
	wideLayoutHeight = 24
)

type layoutMode int

const (
	layoutTiny layoutMode = iota
	layoutCompact
	layoutWide
)

// layoutFor makes a single layout decision shared by every page, including
// Work. Compact is the intentional single-column mode for medium terminals.
func layoutFor(width, height int) layoutMode {
	if width < minLayoutWidth || height < minLayoutHeight {
		return layoutTiny
	}
	if width >= wideLayoutWidth && height >= wideLayoutHeight {
		return layoutWide
	}
	return layoutCompact
}
