package security

import (
	"encoding/hex"
	"testing"
)

func TestPBKDF2Vectors(t *testing.T) {
	cases := map[int]string{1: "120fb6cffcf8b32c43e7225256c4f837a86548c92ccc35480805987cb70be17b", 2: "ae4d0c95af6b46d32d0adff928f06dd02a303f8ef3c251dfd6e2d85a95474c43"}
	for n, want := range cases {
		if got := hex.EncodeToString(PBKDF2SHA256([]byte("password"), []byte("salt"), n)); got != want {
			t.Fatalf("%d: %s", n, got)
		}
	}
}
func TestPasswords(t *testing.T) {
	hash, e := HashPassword("Testing-local-password!")
	if e != nil {
		t.Fatal(e)
	}
	if !VerifyPassword("Testing-local-password!", hash) || VerifyPassword("wrong", hash) {
		t.Fatal("verification failed")
	}
	if _, e = HashPassword("short"); e == nil {
		t.Fatal("weak password accepted")
	}
}
