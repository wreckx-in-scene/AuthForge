package normalize

import "testing"

func TestEmail(t *testing.T) {
	display, normalized, err := Email("  Ada@Campus.EDU ")
	if err != nil {
		t.Fatal(err)
	}
	if display != "Ada@Campus.EDU" || normalized != "ada@campus.edu" {
		t.Fatalf("display=%q normalized=%q", display, normalized)
	}
	if _, _, err := Email("not-an-email"); err == nil {
		t.Fatal("expected invalid email")
	}
}

func TestSlug(t *testing.T) {
	got, err := Slug("", "Campus Connect")
	if err != nil || got != "campus-connect" {
		t.Fatalf("slug=%q err=%v", got, err)
	}
	if _, err := Slug("-bad", ""); err == nil {
		t.Fatal("expected invalid slug")
	}
}
