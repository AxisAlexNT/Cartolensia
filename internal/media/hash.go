package media

import (
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"io"
)

type HashResult struct {
	SHA512Hex string `json:"sha512_hex"`
	Bytes     int64  `json:"bytes"`
}

func HashReader(reader io.Reader) (HashResult, error) {
	hash := sha512.New()
	n, err := io.Copy(hash, reader)
	if err != nil {
		return HashResult{}, fmt.Errorf("hash stream: %w", err)
	}
	return HashResult{SHA512Hex: hex.EncodeToString(hash.Sum(nil)), Bytes: n}, nil
}
