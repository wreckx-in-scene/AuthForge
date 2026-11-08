package server

import (
	"fmt"
	"testing"
)

func TestNormalizeRedirectURIs(t *testing.T) {
	tests := []struct {
		name        string
		input       []string
		wantCount   int
		wantFailure bool
	}{
		{
			name:      "HTTPS callback",
			input:     []string{"https://app.example.com/auth/callback?source=web"},
			wantCount: 1,
		},
		{
			name:      "loopback development callback",
			input:     []string{"http://127.0.0.2:3000/callback"},
			wantCount: 1,
		},
		{
			name:      "duplicate callbacks",
			input:     []string{"https://app.example.com/callback", "https://app.example.com/callback"},
			wantCount: 1,
		},
		{
			name:        "non-loopback HTTP",
			input:       []string{"http://app.example.com/callback"},
			wantFailure: true,
		},
		{
			name:        "callback with user info",
			input:       []string{"https://user@app.example.com/callback"},
			wantFailure: true,
		},
		{
			name:        "callback with fragment",
			input:       []string{"https://app.example.com/callback#token"},
			wantFailure: true,
		},
		{
			name:        "callback without hostname",
			input:       []string{"https://:443/callback"},
			wantFailure: true,
		},
		{
			name:        "callback with unescaped whitespace",
			input:       []string{"https://app.example.com/callback path"},
			wantFailure: true,
		},
		{
			name:        "missing callback",
			input:       nil,
			wantFailure: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			uris, err := normalizeRedirectURIs(test.input)
			if test.wantFailure {
				if err == nil {
					t.Fatal("expected invalid redirect URI input to fail")
				}
				return
			}
			if err != nil {
				t.Fatalf("normalizeRedirectURIs() error = %v", err)
			}
			if len(uris) != test.wantCount {
				t.Fatalf("normalizeRedirectURIs() returned %d URIs, want %d", len(uris), test.wantCount)
			}
		})
	}
}

func TestNormalizeRedirectURILimit(t *testing.T) {
	uris := make([]string, 11)
	for i := range uris {
		uris[i] = fmt.Sprintf("https://app.example.com/callback/%d", i)
	}
	if _, err := normalizeRedirectURIs(uris); err == nil {
		t.Fatal("expected more than 10 redirect URIs to fail")
	}
}
