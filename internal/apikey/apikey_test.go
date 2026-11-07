package apikey

import (
	"strings"
	"testing"

	"github.com/wreckx-in-scene/AuthForge/internal/digest"
)

func TestGenerate(t *testing.T) {
	plain, prefix, sum, err := Generate("production", "publishable")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(plain, "af_live_pk_") {
		t.Fatalf("plaintext prefix: %s", plain)
	}
	if !strings.HasPrefix(plain, prefix) {
		t.Fatalf("stored prefix %s is not a prefix of the key", prefix)
	}
	if len(sum) != 32 || string(sum) == plain {
		t.Fatal("expected a sha-256 hash")
	}
	if string(digest.SHA256(plain)) != string(sum) {
		t.Fatal("hash did not match the plaintext")
	}

	testKey, _, _, err := Generate("development", "secret")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(testKey, "af_test_sk_") {
		t.Fatalf("development key: %s", testKey)
	}
}
