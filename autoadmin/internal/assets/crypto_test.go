package assets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"strings"
	"testing"

	db "autoadmin/internal/platform/database/generated"
)

func TestEncryptProducesFernetCompatibleToken(t *testing.T) {
	encryptor, err := newSecretEncryptor("", "django-secret")
	if err != nil {
		t.Fatal(err)
	}
	encrypted, err := encryptor.Encrypt("password-123")
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := base64.URLEncoding.DecodeString(strings.TrimPrefix(encrypted, encryptedPrefix))
	if err != nil {
		t.Fatal(err)
	}
	message, signature := decoded[:len(decoded)-sha256.Size], decoded[len(decoded)-sha256.Size:]
	mac := hmac.New(sha256.New, encryptor.key[:16])
	_, _ = mac.Write(message)
	if !hmac.Equal(signature, mac.Sum(nil)) {
		t.Fatal("invalid Fernet signature")
	}
	block, err := aes.NewCipher(encryptor.key[16:])
	if err != nil {
		t.Fatal(err)
	}
	plaintext := make([]byte, len(message)-25)
	cipher.NewCBCDecrypter(block, message[9:25]).CryptBlocks(plaintext, message[25:])
	padding := int(plaintext[len(plaintext)-1])
	if got := string(plaintext[:len(plaintext)-padding]); got != "password-123" {
		t.Fatalf("plaintext = %q", got)
	}
}

func TestCredentialMasksSecrets(t *testing.T) {
	row := dbCredential("enc:v1:ciphertext", "private-key")
	result := credential(row)
	if result.Password == nil || *result.Password != secretMask {
		t.Fatal("password was not masked")
	}
	if result.PrivateKey == nil || *result.PrivateKey != secretMask {
		t.Fatal("private key was not masked")
	}
}

func dbCredential(password, privateKey string) db.AssetsCredential {
	return db.AssetsCredential{Password: sql.NullString{String: password, Valid: true}, PrivateKey: sql.NullString{String: privateKey, Valid: true}}
}
