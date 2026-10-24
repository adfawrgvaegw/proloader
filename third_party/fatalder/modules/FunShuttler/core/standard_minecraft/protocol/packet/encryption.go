package packet

import (
	"bytes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
)

// encrypt holds an encryption session with several fields required to encrypt and/or decrypt incoming
// packets. It may be initialised using secret key bytes computed using the shared secret produced with a
// private and a public ECDSA key.
type encrypt struct {
	sendCounter uint64
	buf         [8]byte
	keyBytes    []byte
	stream      cipher.Stream
	skip        int
}

// newEncrypt returns a new encryption 'session' using the secret key bytes passed. The session has its cipher
// block and IV prepared so that it may be used to decrypt and encrypt data.
func newEncrypt(keyBytes []byte, stream cipher.Stream, skip int) *encrypt {
	return &encrypt{keyBytes: keyBytes, stream: stream, skip: skip}
}

// encrypt encrypts the data passed, adding the packet checksum at the end of it before CFB8 encrypting it.
func (encrypt *encrypt) encrypt(data []byte) []byte {
	// We first write the current send counter to a buffer and use it to produce a packet checksum.
	binary.LittleEndian.PutUint64(encrypt.buf[:], encrypt.sendCounter)
	encrypt.sendCounter++

	// We produce a hash existing of the send counter, packet data and key bytes.
	hash := sha256.New()
	hash.Write(encrypt.buf[:])
	if len(data) > encrypt.skip {
		hash.Write(data[encrypt.skip:])
	}
	hash.Write(encrypt.keyBytes)

	// We add the first 8 bytes of the checksum to the data and encrypt it.
	data = append(data, hash.Sum(nil)[:8]...)

	if len(data) > encrypt.skip {
		encrypt.stream.XORKeyStream(data[encrypt.skip:], data[encrypt.skip:])
	}
	return data
}

// decrypt decrypts the data passed. It does not verify the packet checksum. Verifying the checksum should be
// done using encrypt.verify(data).
func (encrypt *encrypt) decrypt(data []byte) {
	if len(data) > encrypt.skip {
		encrypt.stream.XORKeyStream(data[encrypt.skip:], data[encrypt.skip:])
	}
}

// verify verifies the packet checksum of the decrypted data passed. If successful, nil is returned. Otherwise
// an error is returned describing the invalid checksum.
func (encrypt *encrypt) verify(data []byte) error {
	if len(data) < 8 {
		return fmt.Errorf("encrypted packet must be at least 8 bytes long, got %v", len(data))
	}
	sum := data[len(data)-8:]
	payload := data[:len(data)-8]

	// We first write the current send counter to a buffer and use it to produce a packet checksum.
	binary.LittleEndian.PutUint64(encrypt.buf[:], encrypt.sendCounter)
	encrypt.sendCounter++

	// We produce a hash existing of the send counter, packet data and key bytes.
	hash := sha256.New()
	hash.Write(encrypt.buf[:])
	if len(payload) > encrypt.skip {
		hash.Write(payload[encrypt.skip:])
	}
	hash.Write(encrypt.keyBytes)
	ourSum := hash.Sum(nil)[:8]

	// Finally we check if the original sum was equal to the sum we just produced.
	if !bytes.Equal(sum, ourSum) {
		return fmt.Errorf("invalid checksum of packet %v: expected %x, got %x", encrypt.sendCounter-1, ourSum, sum)
	}
	return nil
}
