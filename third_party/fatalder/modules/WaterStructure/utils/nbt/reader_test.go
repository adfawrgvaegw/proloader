package nbt

import (
	"bytes"
	"testing"
)

func TestTagReader_ReadTagByte(t *testing.T) {
	data := []byte{0x42} // byte value 66
	reader := newOffsetReader(bytes.NewReader(data))
	tagReader := NewTagReader(LittleEndian)

	result, err := tagReader.ReadTagByte(reader)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if result != 0x42 {
		t.Fatalf("Expected 0x42, got 0x%02x", result)
	}
}

func TestTagReader_ReadTagType(t *testing.T) {
	data := []byte{byte(tagByte)} // TAG_Byte
	reader := newOffsetReader(bytes.NewReader(data))
	tagReader := NewTagReader(LittleEndian)

	result, err := tagReader.ReadTagType(reader)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if result != tagByte {
		t.Fatalf("Expected tagByte, got %v", result)
	}
}

func TestTagReader_ReadTagString(t *testing.T) {
	// Little endian string: length (2 bytes) + "Hi" (2 bytes)
	data := []byte{0x02, 0x00, 'H', 'i'}
	reader := newOffsetReader(bytes.NewReader(data))
	tagReader := NewTagReader(LittleEndian)

	result, err := tagReader.ReadTagString(reader)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if result != "Hi" {
		t.Fatalf("Expected 'Hi', got '%s'", result)
	}
}

func TestTagReader_ReadTagByteArray(t *testing.T) {
	// Little endian: length (4 bytes) + data (3 bytes)
	data := []byte{0x03, 0x00, 0x00, 0x00, 0x01, 0x02, 0x03}
	reader := newOffsetReader(bytes.NewReader(data))
	tagReader := NewTagReader(LittleEndian)

	result, err := tagReader.ReadTagByteArray(reader)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	expected := []byte{0x01, 0x02, 0x03}
	if len(result) != len(expected) {
		t.Fatalf("Expected length %d, got %d", len(expected), len(result))
	}
	for i, v := range expected {
		if result[i] != v {
			t.Fatalf("Expected %v at index %d, got %v", v, i, result[i])
		}
	}
}

func TestTagReader_ReadTagByteArray_NegativeLength(t *testing.T) {
	// Little endian: negative length (-1)
	data := []byte{0xFF, 0xFF, 0xFF, 0xFF}
	reader := newOffsetReader(bytes.NewReader(data))
	tagReader := NewTagReader(LittleEndian)

	_, err := tagReader.ReadTagByteArray(reader)
	if err == nil {
		t.Fatal("Expected error for negative length, got nil")
	}
	if _, ok := err.(BufferOverrunError); !ok {
		t.Fatalf("Expected BufferOverrunError, got %T", err)
	}
}

func TestTagReader_ReadTag(t *testing.T) {
	// TAG_Byte with name "test": type (1 byte) + name length (2 bytes) + name (4 bytes)
	data := []byte{
		byte(tagByte), // tag type
		0x04, 0x00,    // name length (4)
		't', 'e', 's', 't', // name "test"
	}
	reader := newOffsetReader(bytes.NewReader(data))
	tagReader := NewTagReader(LittleEndian)

	tagType, tagName, err := tagReader.ReadTag(reader)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if tagType != tagByte {
		t.Fatalf("Expected tagByte, got %v", tagType)
	}
	if tagName != "test" {
		t.Fatalf("Expected 'test', got '%s'", tagName)
	}
}

func TestTagReader_ReadTag_End(t *testing.T) {
	// TAG_End has no name
	data := []byte{byte(tagEnd)}
	reader := newOffsetReader(bytes.NewReader(data))
	tagReader := NewTagReader(LittleEndian)

	tagType, tagName, err := tagReader.ReadTag(reader)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if tagType != tagEnd {
		t.Fatalf("Expected tagEnd, got %v", tagType)
	}
	if tagName != "" {
		t.Fatalf("Expected empty name for TAG_End, got '%s'", tagName)
	}
}

func TestTagReader_ReadTagValue_Byte(t *testing.T) {
	data := []byte{0x42}
	reader := newOffsetReader(bytes.NewReader(data))
	tagReader := NewTagReader(LittleEndian)

	result, err := tagReader.ReadTagValue(reader, tagByte)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if value, ok := result.(byte); !ok || value != 0x42 {
		t.Fatalf("Expected byte 0x42, got %v", result)
	}
}

func TestTagReader_ReadTagValue_End(t *testing.T) {
	reader := newOffsetReader(bytes.NewReader([]byte{}))
	tagReader := NewTagReader(LittleEndian)

	result, err := tagReader.ReadTagValue(reader, tagEnd)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if result != nil {
		t.Fatalf("Expected nil for TAG_End, got %v", result)
	}
}

// Tests for Skip methods

func TestTagReader_SkipTagByte(t *testing.T) {
	data := []byte{0x42, 0x43} // two bytes
	reader := newOffsetReader(bytes.NewReader(data))
	tagReader := NewTagReader(LittleEndian)

	// Skip first byte
	err := tagReader.SkipTagByte(reader)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Read second byte to verify we skipped correctly
	result, err := tagReader.ReadTagByte(reader)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if result != 0x43 {
		t.Fatalf("Expected 0x43, got 0x%02x", result)
	}
}

func TestTagReader_SkipTagString(t *testing.T) {
	// Two strings: "Hi" and "Bye"
	data := []byte{
		0x02, 0x00, 'H', 'i', // first string
		0x03, 0x00, 'B', 'y', 'e', // second string
	}
	reader := newOffsetReader(bytes.NewReader(data))
	tagReader := NewTagReader(LittleEndian)

	// Skip first string
	err := tagReader.SkipTagString(reader)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Read second string to verify we skipped correctly
	result, err := tagReader.ReadTagString(reader)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if result != "Bye" {
		t.Fatalf("Expected 'Bye', got '%s'", result)
	}
}

func TestTagReader_SkipTagByteArray(t *testing.T) {
	// Two byte arrays: [1,2,3] and [4,5]
	data := []byte{
		0x03, 0x00, 0x00, 0x00, 0x01, 0x02, 0x03, // first array
		0x02, 0x00, 0x00, 0x00, 0x04, 0x05, // second array
	}
	reader := newOffsetReader(bytes.NewReader(data))
	tagReader := NewTagReader(LittleEndian)

	// Skip first array
	err := tagReader.SkipTagByteArray(reader)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Read second array to verify we skipped correctly
	result, err := tagReader.ReadTagByteArray(reader)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	expected := []byte{0x04, 0x05}
	if len(result) != len(expected) {
		t.Fatalf("Expected length %d, got %d", len(expected), len(result))
	}
	for i, v := range expected {
		if result[i] != v {
			t.Fatalf("Expected %v at index %d, got %v", v, i, result[i])
		}
	}
}

func TestTagReader_SkipTagList(t *testing.T) {
	// List of bytes: [1, 2, 3] followed by a single byte 0x99
	data := []byte{
		byte(tagByte),          // element type: TAG_Byte
		0x03, 0x00, 0x00, 0x00, // length: 3
		0x01, 0x02, 0x03, // list elements
		0x99, // next byte after list
	}
	reader := newOffsetReader(bytes.NewReader(data))
	tagReader := NewTagReader(LittleEndian)

	// Skip the list
	err := tagReader.SkipTagList(reader)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Read the byte after the list to verify we skipped correctly
	result, err := tagReader.ReadTagByte(reader)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if result != 0x99 {
		t.Fatalf("Expected 0x99, got 0x%02x", result)
	}
}

func TestTagReader_SkipTagCompound(t *testing.T) {
	// Compound with one byte tag, followed by a single byte 0x99
	data := []byte{
		// TAG_Byte named "test"
		byte(tagByte),
		0x04, 0x00, 't', 'e', 's', 't', // name
		0x42, // value
		// TAG_End
		byte(tagEnd),
		// Next byte after compound
		0x99,
	}
	reader := newOffsetReader(bytes.NewReader(data))
	tagReader := NewTagReader(LittleEndian)

	// Skip the compound
	err := tagReader.SkipTagCompound(reader)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Read the byte after the compound to verify we skipped correctly
	result, err := tagReader.ReadTagByte(reader)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if result != 0x99 {
		t.Fatalf("Expected 0x99, got 0x%02x", result)
	}
}

func TestTagReader_SkipTagValue(t *testing.T) {
	// Test skipping different tag types
	tests := []struct {
		name     string
		tagType  tagType
		data     []byte
		expected byte
	}{
		{
			name:     "Skip TAG_Byte",
			tagType:  tagByte,
			data:     []byte{0x42, 0x99}, // byte to skip + verification byte
			expected: 0x99,
		},
		{
			name:     "Skip TAG_Int16",
			tagType:  tagInt16,
			data:     []byte{0x12, 0x34, 0x99}, // int16 to skip + verification byte
			expected: 0x99,
		},
		{
			name:     "Skip TAG_String",
			tagType:  tagString,
			data:     []byte{0x02, 0x00, 'H', 'i', 0x99}, // string to skip + verification byte
			expected: 0x99,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := newOffsetReader(bytes.NewReader(tt.data))
			tagReader := NewTagReader(LittleEndian)

			// Skip the tag value
			err := tagReader.SkipTagValue(reader, tt.tagType)
			if err != nil {
				t.Fatalf("Expected no error, got %v", err)
			}

			// Read verification byte
			result, err := tagReader.ReadTagByte(reader)
			if err != nil {
				t.Fatalf("Expected no error, got %v", err)
			}
			if result != tt.expected {
				t.Fatalf("Expected 0x%02x, got 0x%02x", tt.expected, result)
			}
		})
	}
}

func TestTagReader_SkipTag(t *testing.T) {
	// Complete tag: TAG_Byte named "test" with value 0x42, followed by verification byte 0x99
	data := []byte{
		byte(tagByte),                  // tag type
		0x04, 0x00, 't', 'e', 's', 't', // tag name "test"
		0x42, // tag value
		0x99, // verification byte
	}
	reader := newOffsetReader(bytes.NewReader(data))
	tagReader := NewTagReader(LittleEndian)

	// Skip the complete tag
	err := tagReader.SkipTag(reader)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Read verification byte
	result, err := tagReader.ReadTagByte(reader)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if result != 0x99 {
		t.Fatalf("Expected 0x99, got 0x%02x", result)
	}
}

func TestTagReader_SkipTag_End(t *testing.T) {
	// TAG_End followed by verification byte
	data := []byte{byte(tagEnd), 0x99}
	reader := newOffsetReader(bytes.NewReader(data))
	tagReader := NewTagReader(LittleEndian)

	// Skip TAG_End
	err := tagReader.SkipTag(reader)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Read verification byte
	result, err := tagReader.ReadTagByte(reader)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if result != 0x99 {
		t.Fatalf("Expected 0x99, got 0x%02x", result)
	}
}

func TestTagReader_SkipTagInt32Array(t *testing.T) {
	// Int32 array [1, 2] followed by verification byte 0x99
	data := []byte{
		0x02, 0x00, 0x00, 0x00, // length: 2
		0x01, 0x00, 0x00, 0x00, // first int32: 1
		0x02, 0x00, 0x00, 0x00, // second int32: 2
		0x99, // verification byte
	}
	reader := newOffsetReader(bytes.NewReader(data))
	tagReader := NewTagReader(LittleEndian)

	// Skip the int32 array
	err := tagReader.SkipTagInt32Array(reader)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Read verification byte
	result, err := tagReader.ReadTagByte(reader)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if result != 0x99 {
		t.Fatalf("Expected 0x99, got 0x%02x", result)
	}
}

func TestTagReader_SkipTagInt64Array(t *testing.T) {
	// Int64 array [1] followed by verification byte 0x99
	data := []byte{
		0x01, 0x00, 0x00, 0x00, // length: 1
		0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // int64: 1
		0x99, // verification byte
	}
	reader := newOffsetReader(bytes.NewReader(data))
	tagReader := NewTagReader(LittleEndian)

	// Skip the int64 array
	err := tagReader.SkipTagInt64Array(reader)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Read verification byte
	result, err := tagReader.ReadTagByte(reader)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if result != 0x99 {
		t.Fatalf("Expected 0x99, got 0x%02x", result)
	}
}

func TestTagReader_SkipLargeData(t *testing.T) {
	// Test skipping large data without allocating memory
	reader := NewTagReader(LittleEndian)

	// Create a large byte array (1MB) followed by a verification byte
	var buf bytes.Buffer
	largeSize := int32(1024 * 1024) // 1MB

	// Write array length
	buf.Write([]byte{
		byte(largeSize), byte(largeSize >> 8), byte(largeSize >> 16), byte(largeSize >> 24),
	})

	// Write large data (all zeros)
	for i := int32(0); i < largeSize; i++ {
		buf.WriteByte(0x00)
	}

	// Write verification byte
	buf.WriteByte(0x99)

	offsetReader := newOffsetReader(&buf)

	// Skip the large byte array - this should not allocate 1MB of memory
	err := reader.SkipTagByteArray(offsetReader)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Read verification byte to ensure we skipped correctly
	result, err := reader.ReadTagByte(offsetReader)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if result != 0x99 {
		t.Fatalf("Expected 0x99, got 0x%02x", result)
	}
}

func TestTagReader_SkipLargeString(t *testing.T) {
	// Test skipping large string without allocating memory
	reader := NewTagReader(LittleEndian)

	// Create a large string (32KB) followed by a verification byte
	var buf bytes.Buffer
	largeSize := int16(16384) // 16KB (safe for int16)

	// Write string length (little endian int16)
	buf.Write([]byte{byte(largeSize), byte(largeSize >> 8)})

	// Write large string data (all 'A')
	for i := int16(0); i < largeSize; i++ {
		buf.WriteByte('A')
	}

	// Write verification byte
	buf.WriteByte(0x99)

	offsetReader := newOffsetReader(&buf)

	// Skip the large string - this should not allocate 16KB of memory
	err := reader.SkipTagString(offsetReader)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Read verification byte to ensure we skipped correctly
	result, err := reader.ReadTagByte(offsetReader)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if result != 0x99 {
		t.Fatalf("Expected 0x99, got 0x%02x", result)
	}
}
