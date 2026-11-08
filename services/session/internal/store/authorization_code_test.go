package store

import "testing"

func TestCodeChallenge(t *testing.T) {
	const verifier = "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	const expected = "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"

	challenge, err := codeChallenge(verifier)
	if err != nil {
		t.Fatalf("codeChallenge() error = %v", err)
	}
	if challenge != expected {
		t.Fatalf("codeChallenge() = %q, want %q", challenge, expected)
	}
	if !validCodeChallenge(challenge) {
		t.Fatal("validCodeChallenge() rejected a valid S256 challenge")
	}
	if validCodeChallenge(challenge + "=") {
		t.Fatal("validCodeChallenge() accepted padded base64")
	}
}

func TestCodeVerifierValidation(t *testing.T) {
	tests := []struct {
		name     string
		verifier string
		want     bool
	}{
		{name: "minimum length", verifier: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", want: true},
		{name: "maximum length", verifier: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", want: true},
		{name: "too short", verifier: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", want: false},
		{name: "too long", verifier: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", want: false},
		{name: "invalid character", verifier: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa/"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := validCodeVerifier(test.verifier); got != test.want {
				t.Fatalf("validCodeVerifier() = %t, want %t", got, test.want)
			}
		})
	}
}
