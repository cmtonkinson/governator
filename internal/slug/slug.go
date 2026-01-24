package slug

import "strings"

// Slugify converts the provided text to a lowercase ASCII slug with hyphens.
func Slugify(text string) string {
	clean := strings.TrimSpace(text)
	if clean == "" {
		return ""
	}

	var builder strings.Builder
	builder.Grow(len(clean))
	prevHyphen := false
	for _, r := range strings.ToLower(clean) {
		switch {
		case r >= 'a' && r <= 'z':
			builder.WriteRune(r)
			prevHyphen = false
		case r >= '0' && r <= '9':
			builder.WriteRune(r)
			prevHyphen = false
		default:
			if !prevHyphen {
				builder.WriteRune('-')
				prevHyphen = true
			}
		}
	}

	return strings.Trim(builder.String(), "-")
}
