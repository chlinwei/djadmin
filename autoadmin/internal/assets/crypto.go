package assets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"time"
)

const encryptedPrefix = "enc:v1:"

type secretEncryptor struct{ key []byte }

func newSecretEncryptor(configuredKey, djangoSecret string) (*secretEncryptor, error) {
	var key []byte
	var err error
	if configuredKey != "" {
		key, err = base64.URLEncoding.DecodeString(configuredKey)
	} else {
		digest := sha256.Sum256([]byte(djangoSecret))
		key = digest[:]
	}
	if err != nil || len(key) != 32 {
		return nil, fmt.Errorf("ASSETS_CREDENTIAL_ENCRYPTION_KEY must be a Fernet-compatible 32-byte URL-safe base64 key")
	}
	return &secretEncryptor{key: key}, nil
}

func (encryptor *secretEncryptor) Encrypt(value string) (string, error) {
	if value == "" || len(value) >= len(encryptedPrefix) && value[:len(encryptedPrefix)] == encryptedPrefix {
		return value, nil
	}
	block, err := aes.NewCipher(encryptor.key[16:])
	if err != nil {
		return "", err
	}
	iv := make([]byte, aes.BlockSize)
	if _, err = rand.Read(iv); err != nil {
		return "", fmt.Errorf("generate credential IV: %w", err)
	}
	padding := aes.BlockSize - len(value)%aes.BlockSize
	plaintext := append([]byte(value), make([]byte, padding)...)
	for index := len(plaintext) - padding; index < len(plaintext); index++ {
		plaintext[index] = byte(padding)
	}
	token := make([]byte, 1+8+aes.BlockSize+len(plaintext))
	token[0] = 0x80
	binary.BigEndian.PutUint64(token[1:9], uint64(time.Now().Unix()))
	copy(token[9:25], iv)
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(token[25:], plaintext)
	signature := hmac.New(sha256.New, encryptor.key[:16])
	_, _ = signature.Write(token)
	token = append(token, signature.Sum(nil)...)
	return encryptedPrefix + base64.URLEncoding.EncodeToString(token), nil
}
