package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
)

var (
	ErrInvalidKey        = errors.New("key must be 32 bytes for AES-256")
	ErrCiphertextTooShort = errors.New("ciphertext too short")
	ErrDecryptionFailed  = errors.New("decryption failed: invalid ciphertext or wrong key")
)

func Encrypt(plaintext string, key []byte) (string, error) {

	if len(key) != 32 {
		return "", ErrInvalidKey
	}

	block, err := aes.NewCipher(key)

	if err != nil {
		return "", fmt.Errorf("creating cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)

	if err != nil {
		return "", fmt.Errorf("creating GCM: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())

	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("generating nonce: %w", err)
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)

	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

func Decrypt(ciphertext string, key []byte) (string, error) {

	if len(key) != 32 {
		return "", ErrInvalidKey
	}

	data, err := base64.StdEncoding.DecodeString(ciphertext)

	if err != nil {
		return "", fmt.Errorf("decoding base64: %w", err)
	}

	block, err := aes.NewCipher(key)

	if err != nil {
		return "", fmt.Errorf("creating cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)

	if err != nil {
		return "", fmt.Errorf("creating GCM: %w", err)
	}

	nonceSize := gcm.NonceSize()

	if len(data) < nonceSize {
		return "", ErrCiphertextTooShort
	}

	nonce, ciphertextBytes := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertextBytes, nil)

	if err != nil {
		return "", ErrDecryptionFailed
	}

	return string(plaintext), nil
}
