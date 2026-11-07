package apikey

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"

	"github.com/wreckx-in-scene/AuthForge/internal/digest"
	"github.com/wreckx-in-scene/AuthForge/internal/errs"
)

func Generate(environmentName, class string) (plain, prefix string, sum []byte, err error) {
	kind := ""
	switch class {
	case "publishable":
		kind = "pk"
	case "secret":
		kind = "sk"
	default:
		return "", "", nil, errs.ErrInvalid
	}
	mode := "test"
	if environmentName == "production" {
		mode = "live"
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", "", nil, fmt.Errorf("api key: %w", err)
	}
	plain = fmt.Sprintf("af_%s_%s_%s", mode, kind, base64.RawURLEncoding.EncodeToString(raw))
	prefix = plain
	if len(prefix) > 16 {
		prefix = prefix[:16]
	}
	return plain, prefix, digest.SHA256(plain), nil
}
