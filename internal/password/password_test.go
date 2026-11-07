package password

import "testing"

func TestHashAndVerify(t *testing.T) {
	encoded, err := Hash("correct-horse")
	if err != nil {
		t.Fatal(err)
	}
	ok, err := Verify("correct-horse", encoded)
	if err != nil || !ok {
		t.Fatalf("verify matching password: ok=%v err=%v", ok, err)
	}
	ok, err = Verify("wrong-horse", encoded)
	if err != nil || ok {
		t.Fatalf("verify wrong password: ok=%v err=%v", ok, err)
	}

	other, err := Hash("correct-horse")
	if err != nil {
		t.Fatal(err)
	}
	if encoded == other {
		t.Fatal("expected a unique salt")
	}
}

func TestRejectsShortPassword(t *testing.T) {
	if _, err := Hash("short"); err == nil {
		t.Fatal("expected short password to be rejected")
	}
}

func TestRejectsWeakParameters(t *testing.T) {
	encoded := "$argon2id$v=19$m=8,t=1,p=1$c2FsdHNhbHRzYWx0c2FsdA$bm90YXJlYWxoYXNoYm9keQ"
	if _, err := Verify("correct-horse", encoded); err == nil {
		t.Fatal("expected tiny argon2 parameters to be rejected")
	}
}
