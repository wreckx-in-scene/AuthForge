package normalize

import (
	"strings"

	"github.com/wreckx-in-scene/AuthForge/internal/errs"
)

func Email(raw string) (display, normalized string, err error) {
	display = strings.TrimSpace(raw)
	normalized = strings.ToLower(display)
	if len(normalized) > 320 || strings.Count(normalized, "@") != 1 {
		return "", "", errs.ErrInvalid
	}
	local, domain, _ := strings.Cut(normalized, "@")
	if local == "" || len(domain) < 3 || !strings.Contains(domain, ".") || strings.HasPrefix(domain, ".") || strings.HasSuffix(domain, ".") {
		return "", "", errs.ErrInvalid
	}
	return display, normalized, nil
}

func Slug(requested, fallbackName string) (string, error) {
	candidate := strings.TrimSpace(strings.ToLower(requested))
	if candidate == "" {
		candidate = slugify(fallbackName)
	}
	if !validSlug(candidate) {
		return "", errs.ErrInvalid
	}
	return candidate, nil
}

func slugify(name string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(strings.TrimSpace(name)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			dash = false
		case r == ' ' || r == '-' || r == '_':
			if b.Len() > 0 && !dash {
				b.WriteByte('-')
				dash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

func validSlug(value string) bool {
	if len(value) < 1 || len(value) > 48 {
		return false
	}
	dash := false
	for i, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			dash = false
		case r == '-' && i > 0 && !dash:
			dash = true
		default:
			return false
		}
	}
	return !dash
}
