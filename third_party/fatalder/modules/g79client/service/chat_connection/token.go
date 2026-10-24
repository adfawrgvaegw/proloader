package chat_connection

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"
)

func decodeToken(token string) ([]byte, error) {
	seen := make(map[string]struct{})
	candidates := make([][]byte, 0, 4)

	appendCandidate := func(b []byte) {
		if b == nil {
			return
		}
		key := string(b)
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		candidate := make([]byte, len(b))
		copy(candidate, b)
		candidates = append(candidates, candidate)
	}

	if decoded, err := hex.DecodeString(token); err == nil {
		appendCandidate(decoded)
	}
	if decoded, err := base64.StdEncoding.DecodeString(token); err == nil {
		appendCandidate(decoded)
	}
	if decoded, err := base64.RawStdEncoding.DecodeString(token); err == nil {
		appendCandidate(decoded)
	}
	appendCandidate([]byte(token))

	for _, candidate := range candidates {
		if l := len(candidate); l == 16 || l == 24 || l == 32 {
			return candidate, nil
		}
	}

	return nil, fmt.Errorf("chat_connection: 无法根据 token 推导有效密钥长度 (%d)", len(token))
}
