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
	"strings"
	"time"
)

const encryptedPrefix = "enc:v1:"

type SecretEncryptor struct{ key []byte }

type secretEncryptor = SecretEncryptor

func NewSecretEncryptor(configuredKey, djangoSecret string) (*SecretEncryptor, error) {
	return newSecretEncryptor(configuredKey, djangoSecret)
}

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

func (encryptor *SecretEncryptor) Decrypt(value string) (string, error) {
	if value == "" || !strings.HasPrefix(value, encryptedPrefix) {
		return value, nil
	}
	token, err := base64.URLEncoding.DecodeString(strings.TrimPrefix(value, encryptedPrefix))
	if err != nil || len(token) < 1+8+aes.BlockSize+aes.BlockSize+sha256.Size || token[0] != 0x80 {
		return "", fmt.Errorf("invalid encrypted credential")
	}
	message, suppliedMAC := token[:len(token)-sha256.Size], token[len(token)-sha256.Size:]
	signature := hmac.New(sha256.New, encryptor.key[:16])
	_, _ = signature.Write(message)
	if !hmac.Equal(signature.Sum(nil), suppliedMAC) {
		return "", fmt.Errorf("encrypted credential authentication failed")
	}
	ciphertext := message[25:]
	if len(ciphertext) == 0 || len(ciphertext)%aes.BlockSize != 0 {
		return "", fmt.Errorf("invalid encrypted credential payload")
	}
	block, err := aes.NewCipher(encryptor.key[16:])
	if err != nil {
		return "", err
	}
	plaintext := make([]byte, len(ciphertext))
	cipher.NewCBCDecrypter(block, message[9:25]).CryptBlocks(plaintext, ciphertext)
	padding := int(plaintext[len(plaintext)-1])
	if padding < 1 || padding > aes.BlockSize || padding > len(plaintext) {
		return "", fmt.Errorf("invalid encrypted credential padding")
	}
	for _, value := range plaintext[len(plaintext)-padding:] {
		if int(value) != padding {
			return "", fmt.Errorf("invalid encrypted credential padding")
		}
	}
	return string(plaintext[:len(plaintext)-padding]), nil
}
