package mpay

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

func randomBytes(length int) ([]byte, error) {
	if length <= 0 {
		return []byte{}, nil
	}
	buf := make([]byte, length)
	if _, err := rand.Read(buf); err != nil {
		return nil, err
	}
	return buf, nil
}

func randomHex(byteLen int) (string, error) {
	bytes, err := randomBytes(byteLen)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

func randomMacLowerCase() (string, error) {
	bytes, err := randomBytes(6)
	if err != nil {
		return "", err
	}
	bytes[0] = (bytes[0] | 0x02) & 0xFE
	return fmt.Sprintf("%02x:%02x:%02x:%02x:%02x:%02x", bytes[0], bytes[1], bytes[2], bytes[3], bytes[4], bytes[5]), nil
}
