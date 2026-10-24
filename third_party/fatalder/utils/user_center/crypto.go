package user_center

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
)

type envelope struct {
	K   string `json:"k"`
	IV  string `json:"iv"`
	Alg string `json:"alg,omitempty"`
	RSA string `json:"rsa,omitempty"`
	CT  string `json:"ct"`
	Tag string `json:"tag"`
}

func b64urlEncode(b []byte) string {
	enc := base64.RawURLEncoding.EncodeToString(b)
	return enc
}

func b64urlDecode(s string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(s)
}

func rsaEncryptOAEP(pub *rsa.PublicKey, data []byte) ([]byte, error) {
	return rsa.EncryptOAEP(sha256.New(), rand.Reader, pub, data, nil)
}

func rsaDecryptOAEP(priv *rsa.PrivateKey, data []byte) ([]byte, error) {
	return rsa.DecryptOAEP(sha256.New(), rand.Reader, priv, data, nil)
}

func aesGCMEncrypt(key, plaintext []byte) (iv, ct, tag []byte, err error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, nil, nil, err
	}
	iv = make([]byte, 12)
	if _, err = io.ReadFull(rand.Reader, iv); err != nil {
		return nil, nil, nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, nil, err
	}
	ciphertext := gcm.Seal(nil, iv, plaintext, nil)
	// GCM outputs ct||tag, split
	ct = ciphertext[:len(ciphertext)-gcm.Overhead()]
	tag = ciphertext[len(ciphertext)-gcm.Overhead():]
	return iv, ct, tag, nil
}

func aesGCMDecrypt(key, iv, ct, tag []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	buf := make([]byte, 0, len(ct)+len(tag))
	buf = append(buf, ct...)
	buf = append(buf, tag...)
	return gcm.Open(nil, iv, buf, nil)
}

func makeEnvelope(plaintext []byte, serverPub *rsa.PublicKey) ([]byte, error) {
	key := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, err
	}
	iv, ct, tag, err := aesGCMEncrypt(key, plaintext)
	if err != nil {
		return nil, err
	}
	kEnc, err := rsaEncryptOAEP(serverPub, key)
	if err != nil {
		return nil, err
	}
	env := envelope{
		K:   b64urlEncode(kEnc),
		IV:  b64urlEncode(iv),
		Alg: "AES-256-GCM",
		RSA: "RSA-OAEP-SHA256",
		CT:  b64urlEncode(ct),
		Tag: b64urlEncode(tag),
	}
	return json.Marshal(env)
}

func parseEnvelope(body []byte) (*envelope, error) {
	var env envelope
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, err
	}
	if env.K == "" || env.IV == "" || env.CT == "" || env.Tag == "" {
		return nil, errors.New("invalid envelope fields")
	}
	return &env, nil
}

func openEnvelope(body []byte, clientPriv *rsa.PrivateKey) ([]byte, error) {
	env, err := parseEnvelope(body)
	if err != nil {
		return nil, err
	}
	kEnc, err := b64urlDecode(env.K)
	if err != nil {
		return nil, err
	}
	iv, err := b64urlDecode(env.IV)
	if err != nil {
		return nil, err
	}
	ct, err := b64urlDecode(env.CT)
	if err != nil {
		return nil, err
	}
	tag, err := b64urlDecode(env.Tag)
	if err != nil {
		return nil, err
	}
	key, err := rsaDecryptOAEP(clientPriv, kEnc)
	if err != nil {
		return nil, err
	}
	return aesGCMDecrypt(key, iv, ct, tag)
}
