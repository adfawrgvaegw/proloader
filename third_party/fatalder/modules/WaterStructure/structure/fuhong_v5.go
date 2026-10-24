package structure

import (
	"bytes"
	"compress/zlib"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/TriM-Organization/bedrock-world-operator/world"
	"github.com/Yeah114/WaterStructure/define"
)

const fuHongV5Key = "FuHongBuild"

type FuHongV5 struct {
	FuHongV4
}

func (f *FuHongV5) ID() uint8 {
	return IDFuHongV5
}

func (f *FuHongV5) Name() string {
	return NameFuHongV5
}

func (f *FuHongV5) FromFile(file *os.File) error {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("重置文件指针失败: %w", err)
	}

	payload, err := io.ReadAll(file)
	if err != nil {
		return fmt.Errorf("读取 FuHong V5 文件失败: %w", err)
	}

	jsonText, err := decodeFuHongV5(payload)
	if err != nil {
		return err
	}

	var root struct {
		FuHongBuild []map[string]any `json:"FuHongBuild"`
		BlocksList  []string         `json:"BlocksList"`
	}

	if err := json.NewDecoder(strings.NewReader(jsonText)).Decode(&root); err != nil {
		return fmt.Errorf("解析 FuHong V5 的 JSON 失败: %w", err)
	}

	if len(root.BlocksList) == 0 {
		return ErrInvalidFile
	}

	f.file = file
	f.palette = root.BlocksList
	return f.populateFromBuild(root.FuHongBuild)
}

func decodeFuHongV5(data []byte) (string, error) {
	zr, err := zlib.NewReader(bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("解压 FuHong V5 负载失败: %w", err)
	}
	defer zr.Close()

	encrypted, err := io.ReadAll(zr)
	if err != nil {
		return "", fmt.Errorf("读取 FuHong V5 负载失败: %w", err)
	}

	src := string(encrypted)
	if len(src) == 0 {
		return "", ErrInvalidFile
	}

	var builder strings.Builder
	builder.Grow(len(src))

	keyLen := len(fuHongV5Key)
	if keyLen == 0 {
		return "", fmt.Errorf("FuHong V5 密钥为空")
	}

	runeIndex := 0
	for _, r := range src {
		val := int(r) - (runeIndex % 3)
		val ^= int(fuHongV5Key[runeIndex%keyLen])
		if val < 0 || val > utf8.MaxRune {
			return "", fmt.Errorf("FuHong V5: 解码字符无效: %d", val)
		}
		builder.WriteRune(rune(val))
		runeIndex++
	}

	return builder.String(), nil
}

func (f *FuHongV5) FromMCWorld(
	world *world.BedrockWorld,
	target *os.File,
	point1BlockPos define.BlockPos,
	point2BlockPos define.BlockPos,
	startCallback func(int),
	progressCallback func(),
) error {
	if _, err := target.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("重置目标文件指针失败: %w", err)
	}
	if err := target.Truncate(0); err != nil {
		return fmt.Errorf("清空目标文件失败: %w", err)
	}

	zw := zlib.NewWriter(target)
	ew, err := newFuHongV5EncryptWriter(zw)
	if err != nil {
		_ = zw.Close()
		return err
	}

	if err := f.FuHongV4.WriteTo(world, ew, point1BlockPos, point2BlockPos, startCallback, progressCallback); err != nil {
		_ = ew.Close()
		_ = zw.Close()
		return err
	}

	if err := ew.Close(); err != nil {
		_ = zw.Close()
		return err
	}
	if err := zw.Close(); err != nil {
		return fmt.Errorf("关闭 FuHong V5 压缩流失败: %w", err)
	}

	return nil
}

func encodeFuHongV5(plain string) (string, error) {
	if plain == "" {
		return "", ErrInvalidFile
	}

	if len(fuHongV5Key) == 0 {
		return "", fmt.Errorf("FuHong V5 密钥为空")
	}

	var buf bytes.Buffer
	ew, err := newFuHongV5EncryptWriter(&buf)
	if err != nil {
		return "", err
	}
	if _, err := io.WriteString(ew, plain); err != nil {
		return "", err
	}
	if err := ew.Close(); err != nil {
		return "", err
	}
	return buf.String(), nil
}

type fuHongV5EncryptWriter struct {
	w         io.Writer
	runeIndex int
	buf       []byte
}

func newFuHongV5EncryptWriter(w io.Writer) (*fuHongV5EncryptWriter, error) {
	if w == nil {
		return nil, fmt.Errorf("FuHong V5 writer 为 nil")
	}
	if len(fuHongV5Key) == 0 {
		return nil, fmt.Errorf("FuHong V5 密钥为空")
	}
	return &fuHongV5EncryptWriter{w: w}, nil
}

func (e *fuHongV5EncryptWriter) Write(p []byte) (int, error) {
	if e == nil || e.w == nil {
		return 0, fmt.Errorf("FuHong V5 writer 未初始化")
	}
	if len(p) == 0 {
		return 0, nil
	}

	e.buf = append(e.buf, p...)

	for len(e.buf) > 0 {
		r, size := utf8.DecodeRune(e.buf)
		if r == utf8.RuneError && size == 1 {
			break
		}

		val := int(r) ^ int(fuHongV5Key[e.runeIndex%len(fuHongV5Key)])
		val += e.runeIndex % 3
		if val < 0 || val > utf8.MaxRune {
			return 0, fmt.Errorf("FuHong V5: 编码字符无效: %d", val)
		}

		var out [utf8.UTFMax]byte
		n := utf8.EncodeRune(out[:], rune(val))
		if _, err := e.w.Write(out[:n]); err != nil {
			return 0, err
		}

		e.runeIndex++
		e.buf = e.buf[size:]
	}

	// 对调用者来说，输入字节已被“消费”（未完整 UTF-8 的部分会缓存到下次 Write）。
	return len(p), nil
}

func (e *fuHongV5EncryptWriter) Close() error {
	if e == nil {
		return nil
	}
	if len(e.buf) != 0 {
		return fmt.Errorf("FuHong V5: 输入不是有效 UTF-8")
	}
	return nil
}

func (e *fuHongV5EncryptWriter) Flush() error {
	if e == nil || e.w == nil {
		return nil
	}
	if flusher, ok := e.w.(interface{ Flush() error }); ok {
		return flusher.Flush()
	}
	return nil
}
