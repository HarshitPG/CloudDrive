package utils

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
)

func ComputeSHA256(r io.Reader) (string, error) {
	h := sha256.New()
	_, err := io.Copy(h, r)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
