package crypto

import (
	"crypto/rand"
)

func RandomBytes(n uint) ([]byte, error) {
	b := make([]byte, n)

	if _, err := rand.Read(b); err != nil {
		return nil, err
	}

	return b, nil
}
