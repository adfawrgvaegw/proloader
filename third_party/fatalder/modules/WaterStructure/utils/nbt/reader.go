package nbt

import (
	"io"
)

// offsetReader is a wrapper around an io.Reader, used to track the offset (amount of bytes read) of the data
// that is being read, so that errors may have offset data.
type offsetReader struct {
	io.Reader
	off int64

	// ReadByte is a function provided by offsetReader if the io.Reader does not implement io.ByteReader.
	ReadByte func() (byte, error)
	// Next is a function provided by offsetReader if the io.Reader does not have a Next method.
	Next func(n int) []byte
}

// newOffsetReader returns a new offset reader for the io.Reader passed, setting the ReadByte and Next
// functions as appropriate for that particular reader.
func newOffsetReader(r io.Reader) *offsetReader {
	reader := &offsetReader{Reader: r}
	if byteReader, ok := r.(io.ByteReader); ok {
		reader.ReadByte = func() (byte, error) {
			reader.off++
			return byteReader.ReadByte()
		}
	} else {
		reader.ReadByte = func() (byte, error) {
			data := make([]byte, 1)
			_, err := io.ReadAtLeast(reader, data, 1)
			return data[0], err
		}
	}
	if r, ok := r.(interface {
		Next(n int) []byte
	}); ok {
		reader.Next = func(n int) []byte {
			data := r.Next(n)
			reader.off += int64(len(data))
			return data
		}
	} else {
		reader.Next = func(n int) []byte {
			data := make([]byte, n)
			_, _ = io.ReadAtLeast(reader, data, n)
			return data
		}
	}
	return reader
}

// Read reads from the io.Reader and increases the reader's offset by exactly n.
func (b *offsetReader) Read(p []byte) (n int, err error) {
	n, err = io.ReadAtLeast(b.Reader, p, len(p))
	b.off += int64(n)
	return
}

func (b *offsetReader) GetOffset() int64 {
	return b.off
}

var NewOffsetReader = newOffsetReader

type TagReader struct {
	endian Encoding
}

func NewTagReader(endian Encoding) *TagReader {
	return &TagReader{endian: endian}
}

func (r *TagReader) ReadTagType(reader *offsetReader) (tagType, error) {
	tagTypeByte, err := reader.ReadByte()
	if err != nil {
		return 0, err
	}
	return tagType(tagTypeByte), nil
}

func (r *TagReader) ReadTagByte(reader *offsetReader) (byte, error) {
	return reader.ReadByte()
}

func (r *TagReader) ReadTagInt16(reader *offsetReader) (int16, error) {
	return r.endian.Int16(reader)
}

func (r *TagReader) ReadTagInt32(reader *offsetReader) (int32, error) {
	return r.endian.Int32(reader)
}

func (r *TagReader) ReadTagInt64(reader *offsetReader) (int64, error) {
	return r.endian.Int64(reader)
}

func (r *TagReader) ReadTagFloat32(reader *offsetReader) (float32, error) {
	return r.endian.Float32(reader)
}

func (r *TagReader) ReadTagFloat64(reader *offsetReader) (float64, error) {
	return r.endian.Float64(reader)
}

func (r *TagReader) ReadTagString(reader *offsetReader) (string, error) {
	return r.endian.String(reader)
}

func (r *TagReader) ReadTagByteArray(reader *offsetReader) ([]byte, error) {
	length, err := r.endian.Int32(reader)
	if err != nil {
		return nil, err
	}
	if length < 0 {
		return nil, BufferOverrunError{Op: "ReadTagByteArray"}
	}
	data := make([]byte, length)
	if _, err := reader.Read(data); err != nil {
		return nil, BufferOverrunError{Op: "ReadTagByteArray"}
	}
	return data, nil
}

func (r *TagReader) ReadTagInt32Array(reader *offsetReader) ([]int32, error) {
	return r.endian.Int32Slice(reader)
}

func (r *TagReader) ReadTagInt64Array(reader *offsetReader) ([]int64, error) {
	return r.endian.Int64Slice(reader)
}

// ReadTag reads a complete NBT tag including its type and name (if applicable)
func (r *TagReader) ReadTag(reader *offsetReader) (tagType, string, error) {
	// Read tag type
	tagTypeByte, err := reader.ReadByte()
	if err != nil {
		return 0, "", BufferOverrunError{Op: "ReadTag"}
	}

	t := tagType(tagTypeByte)
	if !t.IsValid() {
		return 0, "", UnknownTagError{Off: reader.off, TagType: t, Op: "ReadTag"}
	}

	// TAG_End has no name
	if t == tagEnd {
		return t, "", nil
	}

	// Read tag name for all other tag types
	tagName, err := r.endian.String(reader)
	if err != nil {
		return 0, "", err
	}

	return t, tagName, nil
}

// ReadTagValue reads the value of a tag based on its type
func (r *TagReader) ReadTagValue(reader *offsetReader, t tagType) (interface{}, error) {
	switch t {
	case tagEnd:
		return nil, nil
	case tagByte:
		return r.ReadTagByte(reader)
	case tagInt16:
		return r.ReadTagInt16(reader)
	case tagInt32:
		return r.ReadTagInt32(reader)
	case tagInt64:
		return r.ReadTagInt64(reader)
	case tagFloat32:
		return r.ReadTagFloat32(reader)
	case tagFloat64:
		return r.ReadTagFloat64(reader)
	case tagByteArray:
		return r.ReadTagByteArray(reader)
	case tagString:
		return r.ReadTagString(reader)
	case tagInt32Array:
		return r.ReadTagInt32Array(reader)
	case tagInt64Array:
		return r.ReadTagInt64Array(reader)
	case tagSlice:
		return r.ReadTagList(reader)
	case tagStruct:
		return r.ReadTagCompound(reader)
	default:
		return nil, UnknownTagError{Off: reader.off, TagType: t, Op: "ReadTagValue"}
	}
}

// ReadTagList reads a TAG_List
func (r *TagReader) ReadTagList(reader *offsetReader) ([]interface{}, error) {
	// Read list element type
	elementTypeByte, err := reader.ReadByte()
	if err != nil {
		return nil, BufferOverrunError{Op: "ReadTagList"}
	}

	elementType := tagType(elementTypeByte)
	if !elementType.IsValid() {
		return nil, UnknownTagError{Off: reader.off, TagType: elementType, Op: "ReadTagList"}
	}

	// Read list length
	length, err := r.endian.Int32(reader)
	if err != nil {
		return nil, err
	}

	if length < 0 {
		return nil, BufferOverrunError{Op: "ReadTagList"}
	}

	// Read list elements
	list := make([]interface{}, length)
	for i := int32(0); i < length; i++ {
		value, err := r.ReadTagValue(reader, elementType)
		if err != nil {
			return nil, err
		}
		list[i] = value
	}

	return list, nil
}

// ReadTagCompound reads a TAG_Compound
func (r *TagReader) ReadTagCompound(reader *offsetReader) (map[string]interface{}, error) {
	compound := make(map[string]interface{})

	for {
		// Read next tag
		tagType, tagName, err := r.ReadTag(reader)
		if err != nil {
			return nil, err
		}

		// TAG_End marks the end of the compound
		if tagType == tagEnd {
			break
		}

		// Read tag value
		value, err := r.ReadTagValue(reader, tagType)
		if err != nil {
			return nil, err
		}

		compound[tagName] = value
	}

	return compound, nil
}

// Skip methods - these methods skip over tag data without reading it into memory

// SkipTagByte skips a single byte
func (r *TagReader) SkipTagByte(reader *offsetReader) error {
	_, err := reader.ReadByte()
	return err
}

// SkipTagInt16 skips a 16-bit integer
func (r *TagReader) SkipTagInt16(reader *offsetReader) error {
	_, err := io.CopyN(io.Discard, reader, 2)
	return err
}

// SkipTagInt32 skips a 32-bit integer
func (r *TagReader) SkipTagInt32(reader *offsetReader) error {
	_, err := io.CopyN(io.Discard, reader, 4)
	return err
}

// SkipTagInt64 skips a 64-bit integer
func (r *TagReader) SkipTagInt64(reader *offsetReader) error {
	_, err := io.CopyN(io.Discard, reader, 8)
	return err
}

// SkipTagFloat32 skips a 32-bit float
func (r *TagReader) SkipTagFloat32(reader *offsetReader) error {
	_, err := io.CopyN(io.Discard, reader, 4)
	return err
}

// SkipTagFloat64 skips a 64-bit float
func (r *TagReader) SkipTagFloat64(reader *offsetReader) error {
	_, err := io.CopyN(io.Discard, reader, 8)
	return err
}

// SkipTagString skips a string by reading its length and then skipping that many bytes
func (r *TagReader) SkipTagString(reader *offsetReader) error {
	// Read string length (strings use Int16 for length)
	length, err := r.endian.Int16(reader)
	if err != nil {
		return err
	}
	if length < 0 {
		return BufferOverrunError{Op: "SkipTagString"}
	}
	// Skip the string data using io.CopyN to avoid memory allocation
	_, err = io.CopyN(io.Discard, reader, int64(length))
	return err
}

// SkipTagByteArray skips a byte array by reading its length and then skipping that many bytes
func (r *TagReader) SkipTagByteArray(reader *offsetReader) error {
	// Read array length
	length, err := r.endian.Int32(reader)
	if err != nil {
		return err
	}
	if length < 0 {
		return BufferOverrunError{Op: "SkipTagByteArray"}
	}
	// Skip the array data using io.CopyN to avoid memory allocation
	_, err = io.CopyN(io.Discard, reader, int64(length))
	return err
}

// SkipTagInt32Array skips an int32 array by using the encoding's Int32Slice method but discarding the result
func (r *TagReader) SkipTagInt32Array(reader *offsetReader) error {
	// Note: We use the encoding's method instead of io.CopyN because some encodings
	// (like NetworkLittleEndian) use variable-length integers that can't be skipped
	// with a fixed byte count
	_, err := r.endian.Int32Slice(reader)
	return err
}

// SkipTagInt64Array skips an int64 array by using the encoding's Int64Slice method but discarding the result
func (r *TagReader) SkipTagInt64Array(reader *offsetReader) error {
	// Note: We use the encoding's method instead of io.CopyN because some encodings
	// (like NetworkLittleEndian) use variable-length integers that can't be skipped
	// with a fixed byte count
	_, err := r.endian.Int64Slice(reader)
	return err
}

// SkipTagList skips a TAG_List by reading its element type and length, then skipping all elements
func (r *TagReader) SkipTagList(reader *offsetReader) error {
	// Read list element type
	elementTypeByte, err := reader.ReadByte()
	if err != nil {
		return BufferOverrunError{Op: "SkipTagList"}
	}

	elementType := tagType(elementTypeByte)
	if !elementType.IsValid() {
		return UnknownTagError{Off: reader.off, TagType: elementType, Op: "SkipTagList"}
	}

	// Read list length
	length, err := r.endian.Int32(reader)
	if err != nil {
		return err
	}

	if length < 0 {
		return BufferOverrunError{Op: "SkipTagList"}
	}

	// Skip all list elements
	for i := int32(0); i < length; i++ {
		if err := r.SkipTagValue(reader, elementType); err != nil {
			return err
		}
	}

	return nil
}

// SkipTagCompound skips a TAG_Compound by reading tags until TAG_End is encountered
func (r *TagReader) SkipTagCompound(reader *offsetReader) error {
	for {
		// Read next tag type
		tagTypeByte, err := reader.ReadByte()
		if err != nil {
			return BufferOverrunError{Op: "SkipTagCompound"}
		}

		t := tagType(tagTypeByte)
		if !t.IsValid() {
			return UnknownTagError{Off: reader.off, TagType: t, Op: "SkipTagCompound"}
		}

		// TAG_End marks the end of the compound
		if t == tagEnd {
			break
		}

		// Skip tag name for all other tag types
		if err := r.SkipTagString(reader); err != nil {
			return err
		}

		// Skip tag value
		if err := r.SkipTagValue(reader, t); err != nil {
			return err
		}
	}

	return nil
}

// SkipTagValue skips the value of a tag based on its type
func (r *TagReader) SkipTagValue(reader *offsetReader, t tagType) error {
	switch t {
	case tagEnd:
		return nil
	case tagByte:
		return r.SkipTagByte(reader)
	case tagInt16:
		return r.SkipTagInt16(reader)
	case tagInt32:
		return r.SkipTagInt32(reader)
	case tagInt64:
		return r.SkipTagInt64(reader)
	case tagFloat32:
		return r.SkipTagFloat32(reader)
	case tagFloat64:
		return r.SkipTagFloat64(reader)
	case tagByteArray:
		return r.SkipTagByteArray(reader)
	case tagString:
		return r.SkipTagString(reader)
	case tagInt32Array:
		return r.SkipTagInt32Array(reader)
	case tagInt64Array:
		return r.SkipTagInt64Array(reader)
	case tagSlice:
		return r.SkipTagList(reader)
	case tagStruct:
		return r.SkipTagCompound(reader)
	default:
		return UnknownTagError{Off: reader.off, TagType: t, Op: "SkipTagValue"}
	}
}

// SkipTag skips a complete NBT tag including its type, name (if applicable), and value
func (r *TagReader) SkipTag(reader *offsetReader) error {
	// Read tag type
	tagTypeByte, err := reader.ReadByte()
	if err != nil {
		return BufferOverrunError{Op: "SkipTag"}
	}

	t := tagType(tagTypeByte)
	if !t.IsValid() {
		return UnknownTagError{Off: reader.off, TagType: t, Op: "SkipTag"}
	}

	// TAG_End has no name or value
	if t == tagEnd {
		return nil
	}

	// Skip tag name for all other tag types
	if err := r.SkipTagString(reader); err != nil {
		return err
	}

	// Skip tag value
	return r.SkipTagValue(reader, t)
}
