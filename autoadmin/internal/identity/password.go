package identity

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"strconv"
	"strings"

	"golang.org/x/crypto/pbkdf2"
)

const djangoPBKDF2Algorithm = "pbkdf2_sha256"
const djangoPBKDF2Iterations = 870000

func VerifyPassword(encoded string, password string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 4 || parts[0] != djangoPBKDF2Algorithm {
		return false
	}
	iterations, err := strconv.Atoi(parts[1])
	if err != nil || iterations < 1 {
		return false
	}
	expected, err := base64.StdEncoding.DecodeString(parts[3])
	if err != nil || len(expected) == 0 {
		return false
	}
	actual := pbkdf2.Key([]byte(password), []byte(parts[2]), iterations, len(expected), sha256.New)
	return subtle.ConstantTimeCompare(actual, expected) == 1
}

func HashPassword(password string) (string, error) {
	saltBytes := make([]byte, 16)
	if _, err := rand.Read(saltBytes); err != nil {
		return "", err
	}
	salt := base64.RawURLEncoding.EncodeToString(saltBytes)
	digest := pbkdf2.Key([]byte(password), []byte(salt), djangoPBKDF2Iterations, sha256.Size, sha256.New)
	return djangoPBKDF2Algorithm + "$" + strconv.Itoa(djangoPBKDF2Iterations) + "$" + salt + "$" + base64.StdEncoding.EncodeToString(digest), nil
}
