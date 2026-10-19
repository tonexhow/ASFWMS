package security

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

const Iterations = 600000

// PBKDF2SHA256 derives exactly one SHA-256 block. It is also checked against
// published PBKDF2-HMAC-SHA256 test vectors in password_test.go.
func PBKDF2SHA256(password, salt []byte, iterations int) []byte {
	mac := hmac.New(sha256.New, password)
	mac.Write(salt)
	mac.Write([]byte{0, 0, 0, 1})
	u := mac.Sum(nil)
	out := append([]byte(nil), u...)
	for i := 1; i < iterations; i++ {
		mac.Reset()
		mac.Write(u)
		u = mac.Sum(u[:0])
		for j := range out {
			out[j] ^= u[j]
		}
	}
	return out
}
func HashPassword(password string) (string, error) {
	if len([]rune(password)) < 12 || len([]rune(password)) > 128 {
		return "", errors.New("use a password with 12-128 characters")
	}
	salt := make([]byte, 16)
	if _, e := rand.Read(salt); e != nil {
		return "", e
	}
	hash := PBKDF2SHA256([]byte(password), salt, Iterations)
	return fmt.Sprintf("pbkdf2-sha256$%d$%s$%s", Iterations, hex.EncodeToString(salt), hex.EncodeToString(hash)), nil
}
func VerifyPassword(password, encoded string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 4 || parts[0] != "pbkdf2-sha256" || len(password) > 512 {
		return false
	}
	n, e := strconv.Atoi(parts[1])
	if e != nil || n < 1 || n > 1000000 {
		return false
	}
	salt, e := hex.DecodeString(parts[2])
	if e != nil || len(salt) != 16 {
		return false
	}
	want, e := hex.DecodeString(parts[3])
	if e != nil || len(want) != 32 {
		return false
	}
	got := PBKDF2SHA256([]byte(password), salt, n)
	return subtle.ConstantTimeCompare(got, want) == 1
}
func Token() (string, error) {
	b := make([]byte, 32)
	_, e := rand.Read(b)
	return hex.EncodeToString(b), e
}
func Digest(token string) string { s := sha256.Sum256([]byte(token)); return hex.EncodeToString(s[:]) }
