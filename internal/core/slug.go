package core

import (
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// Slug IDs are "<prefix>_<slug>" and stay lowercase, hyphenated and at most
// 48 characters. Slugs never start or end with a hyphen.
const (
	slugMaxLength = 48
	slugBaseMax   = 40
)

var (
	slugPattern        = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,46}[a-z0-9])?$`)
	lowerULIDPattern   = regexp.MustCompile(`^[0-9a-hjkmnp-tv-z]{26}$`)
	migratedHexPattern = regexp.MustCompile(`^[0-9a-f]{24}$`)
)

// slugFallbacks keeps every kind's auto-generated ID non-empty and readable.
var slugFallbacks = map[string]string{
	"ws":    "workspace",
	"agent": "agent",
	"sess":  "session",
	"task":  "task",
}

func slugFallback(kind string) string {
	if word, ok := slugFallbacks[kind]; ok {
		return word
	}
	return "item"
}

// foldRune folds letters that have no compatibility decomposition to ASCII.
// It mirrors the small table in the accepted plan.
func foldRune(r rune) string {
	switch r {
	case 'ß', 'ẞ':
		return "ss"
	case 'æ', 'Æ':
		return "ae"
	case 'œ', 'Œ':
		return "oe"
	case 'ø', 'Ø':
		return "o"
	case 'ł', 'Ł':
		return "l"
	case 'đ', 'Đ', 'ð', 'Ð':
		return "d"
	case 'þ', 'Þ':
		return "th"
	case 'ı':
		return "i"
	}
	return string(r)
}

// slugify normalizes free-form text into a slug of at most max characters.
// It applies NFKD, drops combining marks, folds the explicit letters, lowercases
// and collapses every non-alphanumeric run into single hyphens before trimming
// and truncating at a hyphen boundary.
func slugify(source string, max int) string {
	if max <= 0 {
		max = slugBaseMax
	}
	decomposed := norm.NFKD.String(source)
	var normalized strings.Builder
	for _, r := range decomposed {
		if unicode.Is(unicode.Mn, r) {
			continue
		}
		normalized.WriteString(foldRune(r))
	}
	var out strings.Builder
	pendingHyphen := false
	for _, r := range strings.ToLower(normalized.String()) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			if pendingHyphen && out.Len() > 0 {
				out.WriteByte('-')
			}
			pendingHyphen = false
			out.WriteRune(r)
			continue
		}
		pendingHyphen = true
	}
	return trimSlug(out.String(), max)
}

// trimSlug trims leading and trailing hyphens and truncates to max characters,
// cutting at the last hyphen at or before the limit when one exists.
func trimSlug(s string, max int) string {
	s = strings.Trim(s, "-")
	if max <= 0 || len(s) <= max {
		return s
	}
	cut := s[:max]
	if i := strings.LastIndex(cut, "-"); i > 0 {
		return strings.Trim(cut[:i], "-")
	}
	return strings.Trim(cut, "-")
}

func reservedSlug(slug string) bool {
	return lowerULIDPattern.MatchString(slug) || migratedHexPattern.MatchString(slug)
}

// baseSlug derives the unsuffixed slug for a kind. An empty or reserved result
// falls back to the kind's word so a reserved shape is never returned as-is.
func baseSlug(kind, source string) string {
	s := slugify(source, slugBaseMax)
	if s == "" || reservedSlug(s) || !slugPattern.MatchString(s) {
		return slugFallback(kind)
	}
	return s
}

// parseExplicitID validates an explicit ID for kind. The prefix is optional:
// both "<kind>_<slug>" and "<slug>" are accepted. A wrong prefix, an invalid
// slug shape or a reserved shape is refused with invalid_id.
func parseExplicitID(kind, value string) (string, error) {
	v := strings.TrimSpace(value)
	if v == "" {
		return "", fail("invalid_id", "id must not be empty")
	}
	if strings.Contains(v, "_") {
		prefix, slug, ok := strings.Cut(v, "_")
		if !ok || prefix != kind {
			return "", fail("invalid_id", "id %q must use the %q prefix", value, kind+"_")
		}
		v = slug
	}
	if !slugPattern.MatchString(v) {
		return "", fail("invalid_id", "id %q must be 1-48 lowercase letters, digits or hyphens and must not start or end with a hyphen", value)
	}
	if reservedSlug(v) {
		return "", fail("invalid_id", "id %q is a reserved identifier shape", value)
	}
	return v, nil
}
