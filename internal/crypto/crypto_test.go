package crypto

import (
	"encoding/hex"
	"testing"
)

func TestEncryptDecrypt(t *testing.T) {
	key, _ := hex.DecodeString("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	plaintext := "my-secret-password"

	encrypted, err := Encrypt(plaintext, key)

	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	if encrypted == plaintext {
		t.Error("Encrypted text should not match plaintext")
	}

	decrypted, err := Decrypt(encrypted, key)

	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}

	if decrypted != plaintext {
		t.Errorf("Decrypted text = %q, want %q", decrypted, plaintext)
	}
}

func TestDecryptWrongKey(t *testing.T) {
	key1, _ := hex.DecodeString("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	key2, _ := hex.DecodeString("fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210")

	encrypted, err := Encrypt("secret", key1)

	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	_, err = Decrypt(encrypted, key2)

	if err != ErrDecryptionFailed {
		t.Errorf("Expected ErrDecryptionFailed, got %v", err)
	}
}

func TestDecryptTamperedCiphertext(t *testing.T) {
	key, _ := hex.DecodeString("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")

	encrypted, err := Encrypt("secret", key)

	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	tampered := encrypted[:len(encrypted)-2] + "XX"
	_, err = Decrypt(tampered, key)

	if err == nil {
		t.Error("Expected error for tampered ciphertext")
	}
}

func TestInvalidKeyLength(t *testing.T) {
	shortKey := []byte("too-short")

	_, err := Encrypt("test", shortKey)

	if err != ErrInvalidKey {
		t.Errorf("Encrypt: expected ErrInvalidKey, got %v", err)
	}

	_, err = Decrypt("dGVzdA==", shortKey)

	if err != ErrInvalidKey {
		t.Errorf("Decrypt: expected ErrInvalidKey, got %v", err)
	}
}

func TestEncryptProducesDifferentOutput(t *testing.T) {
	key, _ := hex.DecodeString("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	plaintext := "same-input"

	encrypted1, _ := Encrypt(plaintext, key)
	encrypted2, _ := Encrypt(plaintext, key)

	if encrypted1 == encrypted2 {
		t.Error("Same plaintext should produce different ciphertext (random nonce)")
	}
}
