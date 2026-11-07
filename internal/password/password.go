package password

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"

	"github.com/wreckx-in-scene/AuthForge/internal/errs"
)

const (
	memory      = 64 * 1024
	iterations  = 3
	parallelism = 4
	saltLen     = 16
	keyLen      = 32
	minLen      = 8
	maxLen      = 256
)

var Dummy string

func init() {
	encoded, err := Hash("authforge-dummy-password")
	if err != nil {
		panic(err)
	}
	Dummy = encoded
}

func Hash(plain string) (string, error) {
	if len(plain) < minLen || len(plain) > maxLen {
		return "", errs.ErrInvalid
	}
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("salt: %w", err)
	}
	sum := argon2.IDKey([]byte(plain), salt, iterations, memory, parallelism, keyLen)
	return fmt.Sprintf(
		"$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s",
		memory,
		iterations,
		parallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(sum),
	), nil
}

func Verify(plain, encoded string) (bool, error) {
	salt, sum, mem, iters, par, err := decode(encoded)
	if err != nil {
		return false, err
	}
	got := argon2.IDKey([]byte(plain), salt, iters, mem, par, uint32(len(sum)))
	if len(got) != len(sum) {
		return false, nil
	}
	return subtle.ConstantTimeCompare(got, sum) == 1, nil
}

func decode(encoded string) (salt, sum []byte, memory uint32, iterations uint32, parallelism uint8, err error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" || parts[2] != "v=19" {
		return nil, nil, 0, 0, 0, fmt.Errorf("malformed password hash")
	}
	var mem, iters int
	var par int
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &mem, &iters, &par); err != nil {
		return nil, nil, 0, 0, 0, fmt.Errorf("malformed password parameters")
	}
	if mem < 8*1024 || mem > 128*1024 || iters < 1 || iters > 5 || par < 1 || par > 4 {
		return nil, nil, 0, 0, 0, fmt.Errorf("password parameters out of range")
	}
	salt, err = base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return nil, nil, 0, 0, 0, fmt.Errorf("malformed password salt")
	}
	sum, err = base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return nil, nil, 0, 0, 0, fmt.Errorf("malformed password digest")
	}
	if len(salt) < 16 || len(sum) < 16 {
		return nil, nil, 0, 0, 0, fmt.Errorf("malformed password hash")
	}
	return salt, sum, uint32(mem), uint32(iters), uint8(par), nil
}
