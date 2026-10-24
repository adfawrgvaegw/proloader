package netease_world

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

var encryptedHeader = []byte{0x80, 0x1d, 0x30, 0x01}

var (
	ErrNotDirectory     = errors.New("not a directory")
	ErrMissingCurrent   = errors.New("missing CURRENT file")
	ErrMissingManifest  = errors.New("missing MANIFEST file")
	ErrNotEncrypted     = errors.New("directory does not look encrypted (CURRENT missing header)")
	ErrAlreadyEncrypted = errors.New("directory already encrypted (CURRENT has header)")
)

// Options controls Encrypt/Decrypt behavior.
type Options struct {
	// BufferSize is the streaming buffer size used for file transforms.
	// When <= 0, a default will be used.
	BufferSize int
}

func normalizeOptions(options *Options) Options {
	if options == nil {
		return Options{BufferSize: 1 << 20}
	}
	if options.BufferSize <= 0 {
		options.BufferSize = 1 << 20
	}
	return *options
}

// Decrypt decrypts all encrypted files directly under dbPath (a LevelDB directory).
// It returns ErrNotEncrypted when CURRENT does not have the expected header.
func Decrypt(dbPath string, options *Options) error {
	optionsValue := normalizeOptions(options)

	dbAbs, err := filepath.Abs(dbPath)
	if err != nil {
		return fmt.Errorf("failed to resolve path: %w", err)
	}
	info, err := os.Stat(dbAbs)
	if err != nil {
		return fmt.Errorf("failed to stat directory: %w", err)
	}
	if !info.IsDir() {
		return ErrNotDirectory
	}

	entries, err := os.ReadDir(dbAbs)
	if err != nil {
		return fmt.Errorf("failed to read directory: %w", err)
	}

	currentPath, manifestName, err := findCurrentAndManifest(dbAbs, entries)
	if err != nil {
		return err
	}

	currentEncrypted, err := fileHasHeader(currentPath)
	if err != nil {
		return fmt.Errorf("failed to read CURRENT header: %w", err)
	}
	if !currentEncrypted {
		return ErrNotEncrypted
	}

	key, err := deriveKeyFromCurrent(currentPath, manifestName)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := filepath.Join(dbAbs, entry.Name())
		ok, err := fileHasHeader(path)
		if err != nil {
			continue
		}
		if !ok {
			continue
		}
		if err := decryptFileInPlace(path, key, optionsValue.BufferSize); err != nil {
			continue
		}
	}

	return nil
}

// Encrypt encrypts all regular files directly under dbPath (a LevelDB directory).
// It returns ErrAlreadyEncrypted when CURRENT already has the expected header.
//
// Key generation is inferred from the Python implementation:
//   - Read plaintext CURRENT to obtain manifest name (usually "MANIFEST-xxxxxx").
//   - Let source = manifestName + "\n".
//   - Generate random key with the same length as source.
//   - Write CURRENT as: header + XOR(key, source).
//   - For all other files: header + XOR(plaintext, key).
func Encrypt(dbPath string, options *Options) error {
	optionsValue := normalizeOptions(options)

	dbAbs, err := filepath.Abs(dbPath)
	if err != nil {
		return fmt.Errorf("failed to resolve path: %w", err)
	}
	info, err := os.Stat(dbAbs)
	if err != nil {
		return fmt.Errorf("failed to stat directory: %w", err)
	}
	if !info.IsDir() {
		return ErrNotDirectory
	}

	entries, err := os.ReadDir(dbAbs)
	if err != nil {
		return fmt.Errorf("failed to read directory: %w", err)
	}

	currentPath, _, err := findCurrentAndManifest(dbAbs, entries)
	if err != nil {
		return err
	}

	currentEncrypted, err := fileHasHeader(currentPath)
	if err != nil {
		return fmt.Errorf("failed to read CURRENT header: %w", err)
	}
	if currentEncrypted {
		return ErrAlreadyEncrypted
	}

	currentPlain, err := os.ReadFile(currentPath)
	if err != nil {
		return fmt.Errorf("failed to read CURRENT: %w", err)
	}
	manifestName := strings.TrimSpace(string(currentPlain))
	if manifestName == "" {
		return fmt.Errorf("CURRENT is empty")
	}
	manifestName = filepath.Base(manifestName)

	if _, err := os.Stat(filepath.Join(dbAbs, manifestName)); err != nil {
		// If CURRENT is unreliable, fall back to scanning the directory (same strategy as Decrypt).
		_, scannedManifestName, scanErr := findCurrentAndManifest(dbAbs, entries)
		if scanErr == nil {
			manifestName = scannedManifestName
		}
	}

	source := append([]byte(manifestName), '\n')
	key := make([]byte, len(source))
	if _, err := rand.Read(key); err != nil {
		return fmt.Errorf("failed to generate random key: %w", err)
	}

	currentBody := xorBytes(key, source)
	if err := writeFileAtomically(currentPath, append(append([]byte{}, encryptedHeader...), currentBody...), 0o644); err != nil {
		return fmt.Errorf("failed to write CURRENT: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if entry.Name() == "CURRENT" {
			continue
		}

		path := filepath.Join(dbAbs, entry.Name())
		ok, err := fileHasHeader(path)
		if err != nil {
			continue
		}
		if ok {
			continue
		}
		if err := encryptFileInPlace(path, key, optionsValue.BufferSize); err != nil {
			continue
		}
	}

	return nil
}

func findCurrentAndManifest(dir string, entries []os.DirEntry) (currentPath string, manifestName string, err error) {
	var current string
	var manifests []string

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if name == "CURRENT" {
			current = filepath.Join(dir, name)
			continue
		}
		if strings.HasPrefix(name, "MANIFEST") {
			manifests = append(manifests, name)
		}
	}

	if current == "" {
		return "", "", ErrMissingCurrent
	}
	if len(manifests) == 0 {
		return "", "", ErrMissingManifest
	}

	sort.Strings(manifests)
	return current, manifests[len(manifests)-1], nil
}

func deriveKeyFromCurrent(currentPath string, manifestName string) ([]byte, error) {
	f, err := os.Open(currentPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open CURRENT: %w", err)
	}
	defer func() { _ = f.Close() }()

	head := make([]byte, len(encryptedHeader))
	if _, err := io.ReadFull(f, head); err != nil {
		return nil, fmt.Errorf("failed to read CURRENT header: %w", err)
	}
	if !bytes.Equal(head, encryptedHeader) {
		return nil, ErrNotEncrypted
	}

	body, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("failed to read CURRENT body: %w", err)
	}
	source := append([]byte(manifestName), '\n')
	return xorBytes(body, source), nil
}

func fileHasHeader(path string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer func() { _ = f.Close() }()

	head := make([]byte, len(encryptedHeader))
	_, err = io.ReadFull(f, head)
	if err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return false, nil
		}
		return false, err
	}
	return bytes.Equal(head, encryptedHeader), nil
}

func decryptFileInPlace(path string, key []byte, bufferSize int) error {
	src, err := os.Open(path)
	if err != nil {
		return err
	}

	head := make([]byte, len(encryptedHeader))
	if _, err := io.ReadFull(src, head); err != nil {
		_ = src.Close()
		return err
	}
	if !bytes.Equal(head, encryptedHeader) {
		_ = src.Close()
		return nil
	}

	dir := filepath.Dir(path)
	base := filepath.Base(path)
	tmp, err := os.CreateTemp(dir, "."+base+".decrypt-*")
	if err != nil {
		_ = src.Close()
		return err
	}
	tmpPath := tmp.Name()
	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
	}

	if err := xorStream(tmp, bufio.NewReaderSize(src, bufferSize), key, bufferSize); err != nil {
		_ = src.Close()
		cleanup()
		return err
	}

	if err := tmp.Close(); err != nil {
		_ = src.Close()
		cleanup()
		return err
	}
	if err := src.Close(); err != nil {
		cleanup()
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		cleanup()
		return err
	}
	return nil
}

func encryptFileInPlace(path string, key []byte, bufferSize int) error {
	src, err := os.Open(path)
	if err != nil {
		return err
	}

	dir := filepath.Dir(path)
	base := filepath.Base(path)
	tmp, err := os.CreateTemp(dir, "."+base+".encrypt-*")
	if err != nil {
		_ = src.Close()
		return err
	}
	tmpPath := tmp.Name()
	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
	}

	if _, err := tmp.Write(encryptedHeader); err != nil {
		_ = src.Close()
		cleanup()
		return err
	}

	// Need to include all plaintext bytes, including any initial bytes we might
	// have read for header checking (we don't do that here; we just stream).
	if err := xorStream(tmp, bufio.NewReaderSize(src, bufferSize), key, bufferSize); err != nil {
		_ = src.Close()
		cleanup()
		return err
	}

	if err := tmp.Close(); err != nil {
		_ = src.Close()
		cleanup()
		return err
	}
	if err := src.Close(); err != nil {
		cleanup()
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		cleanup()
		return err
	}
	return nil
}

func xorStream(dst io.Writer, src io.Reader, key []byte, bufferSize int) error {
	if len(key) == 0 {
		return fmt.Errorf("key length is 0")
	}
	if bufferSize <= 0 {
		bufferSize = 1 << 20
	}
	buf := make([]byte, bufferSize)
	offset := 0
	for {
		n, err := src.Read(buf)
		if n > 0 {
			xorInPlace(buf[:n], key, offset)
			if _, wErr := dst.Write(buf[:n]); wErr != nil {
				return wErr
			}
			offset += n
		}
		if err == nil {
			continue
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
		return err
	}
}

func xorBytes(p []byte, k []byte) []byte {
	if len(p) == 0 {
		return nil
	}
	out := make([]byte, len(p))
	copy(out, p)
	xorInPlace(out, k, 0)
	return out
}

func xorInPlace(buf []byte, key []byte, offset int) {
	if len(key) == 0 {
		return
	}
	for i := range buf {
		buf[i] ^= key[(offset+i)%len(key)]
	}
}

func writeFileAtomically(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	base := filepath.Base(path)
	tmp, err := os.CreateTemp(dir, "."+base+".write-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
	}

	if _, err := tmp.Write(data); err != nil {
		cleanup()
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		cleanup()
		return err
	}
	return nil
}
