package crypto

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
)

var (
	ErrPemDecodeFailed    = errors.New("failed to decode pem data")
	ErrPemKeyDecodeFailed = errors.New("failed to decode pem key")
)

func Sign(message []byte, key *rsa.PrivateKey) ([]byte, error) {
	hashed := sha256.Sum256(message)

	s, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, hashed[:])
	if err != nil {
		return nil, fmt.Errorf("failed to sign message: %w", err)
	}

	return s, nil
}

func Verify(message, signature []byte, key *rsa.PublicKey) error {
	hashed := sha256.Sum256(message)

	return rsa.VerifyPKCS1v15(key, crypto.SHA256, hashed[:], signature)
}

func ParsePublicKey(source io.Reader) (*rsa.PublicKey, error) {
	data, err := io.ReadAll(source)
	if err != nil {
		return nil, fmt.Errorf("failed to read data from source: %w", err)
	}

	p, _ := pem.Decode(data)
	if p == nil {
		return nil, ErrPemDecodeFailed
	}

	if p.Type != "PUBLIC KEY" {
		return nil, fmt.Errorf("public key is not RSA format: %s", p.Type)
	}

	r, err := x509.ParsePKIXPublicKey(p.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse public key: %w", err)
	}

	return r.(*rsa.PublicKey), nil
}

func ParsePrivateKey(source io.Reader) (*rsa.PrivateKey, error) {
	data, err := io.ReadAll(source)
	if err != nil {
		return nil, fmt.Errorf("failed to read data from source: %w", err)
	}

	p, _ := pem.Decode(data)
	if p == nil {
		return nil, ErrPemDecodeFailed
	}

	if p.Type != "RSA PRIVATE KEY" {
		return nil, fmt.Errorf("private key is not RSA format: %s", p.Type)
	}

	r, err := x509.ParsePKCS1PrivateKey(p.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %w", err)
	}

	return r, nil
}
