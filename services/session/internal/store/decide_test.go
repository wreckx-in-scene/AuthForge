package store

import (
	"testing"
	"time"
)

func TestDecide(t *testing.T) {
	now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	future := now.Add(time.Hour)
	past := now.Add(-time.Second)

	if got := decide(true, false, "active", future, future, now); got != decisionReuse {
		t.Fatalf("used token: %v", got)
	}
	if got := decide(false, true, "active", future, future, now); got != decisionReject {
		t.Fatalf("revoked token: %v", got)
	}
	if got := decide(false, false, "revoked", future, future, now); got != decisionReject {
		t.Fatalf("revoked session: %v", got)
	}
	if got := decide(false, false, "active", past, future, now); got != decisionReject {
		t.Fatalf("expired token: %v", got)
	}
	if got := decide(false, false, "active", future, future, now); got != decisionRotate {
		t.Fatalf("fresh token: %v", got)
	}
}
