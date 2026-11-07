package digest

import "crypto/sha256"

func SHA256(value string) []byte {
	sum := sha256.Sum256([]byte(value))
	return sum[:]
}
