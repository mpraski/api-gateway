package token

import (
	"crypto/rsa"
	"encoding/base64"
	"fmt"
	"io"

	"github.com/mpraski/api-gateway/crypto"
)

type (
	Reference struct {
		message   []byte
		signature []byte
	}

	ReferenceIssuer struct {
		privateKey *rsa.PrivateKey
	}

	ReferenceParser struct {
		publicKey *rsa.PublicKey
	}
)

const (
	messageSize   = 16
	signatureSize = 128
)

func (p *Reference) String() string {
	return base64.StdEncoding.EncodeToString(append(p.signature, p.message...))
}

func NewReferenceIssuer(privateKey io.Reader) (*ReferenceIssuer, error) {
	p, err := crypto.ParsePrivateKey(privateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %w", err)
	}

	return &ReferenceIssuer{privateKey: p}, nil
}

func (p *ReferenceIssuer) Issue() (Token, error) {
	m, err := crypto.RandomBytes(messageSize)
	if err != nil {
		return nil, fmt.Errorf("failed to obtains random bytes: %w", err)
	}

	s, err := crypto.Sign(m, p.privateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to sign message: %w", err)
	}

	return &Reference{
		message:   m,
		signature: s,
	}, nil
}

func NewReferenceParser(publicKey io.Reader) (*ReferenceParser, error) {
	p, err := crypto.ParsePublicKey(publicKey)
	if err != nil {
		return nil, fmt.Errorf("failed to parse public key: %w", err)
	}

	return &ReferenceParser{publicKey: p}, nil
}

func (p *ReferenceParser) Parse(data string) (Token, error) {
	b, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return nil, fmt.Errorf("failed to decode base64 data: %w", err)
	}

	if len(b) != signatureSize+messageSize {
		return nil, ErrTokenInvalid
	}

	t := Reference{
		signature: b[:signatureSize],
		message:   b[signatureSize:(signatureSize + messageSize)],
	}

	if err := crypto.Verify(t.message, t.signature, p.publicKey); err != nil {
		return nil, fmt.Errorf("failed to verify reference token: %w", err)
	}

	return &t, nil
}
