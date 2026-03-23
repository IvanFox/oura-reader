package crypto

import (
	"bytes"
	"testing"
)

func TestEncryptDecrypt(t *testing.T) {
	c, err := NewCipher("test-key-12345")
	if err != nil {
		t.Fatal(err)
	}

	plaintext := []byte(`{"access_token":"abc","refresh_token":"xyz"}`)
	encrypted, err := c.Encrypt(plaintext)
	if err != nil {
		t.Fatal(err)
	}

	if bytes.Equal(encrypted, plaintext) {
		t.Fatal("encrypted data should differ from plaintext")
	}

	decrypted, err := c.Decrypt(encrypted)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(decrypted, plaintext) {
		t.Fatalf("got %q, want %q", decrypted, plaintext)
	}
}

func TestDecryptWrongKey(t *testing.T) {
	c1, _ := NewCipher("key-one")
	c2, _ := NewCipher("key-two")

	encrypted, _ := c1.Encrypt([]byte("secret"))
	_, err := c2.Decrypt(encrypted)
	if err == nil {
		t.Fatal("expected error decrypting with wrong key")
	}
}

func TestDecryptTooShort(t *testing.T) {
	c, _ := NewCipher("key")
	_, err := c.Decrypt([]byte("short"))
	if err == nil {
		t.Fatal("expected error for short ciphertext")
	}
}

func TestEncryptProducesDifferentCiphertexts(t *testing.T) {
	c, _ := NewCipher("key")
	plaintext := []byte("same input")

	e1, _ := c.Encrypt(plaintext)
	e2, _ := c.Encrypt(plaintext)

	if bytes.Equal(e1, e2) {
		t.Fatal("two encryptions of same plaintext should produce different ciphertexts (random nonce)")
	}
}
