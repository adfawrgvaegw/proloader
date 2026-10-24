package nbt

import (
	"bytes"
	"fmt"
)

// ExampleTagReader demonstrates how to use TagReader to read NBT data
func ExampleTagReader() {
	// Create some sample NBT data representing a simple compound tag
	// This would typically come from a file or network stream
	var buf bytes.Buffer

	// Write a compound tag with a few fields
	// TAG_Compound (root)
	buf.WriteByte(byte(tagStruct))
	// Root tag name: "root"
	buf.Write([]byte{0x04, 0x00}) // length = 4 (little endian)
	buf.WriteString("root")

	// TAG_Byte named "level"
	buf.WriteByte(byte(tagByte))
	buf.Write([]byte{0x05, 0x00}) // length = 5
	buf.WriteString("level")
	buf.WriteByte(42) // value = 42

	// TAG_String named "name"
	buf.WriteByte(byte(tagString))
	buf.Write([]byte{0x04, 0x00}) // length = 4
	buf.WriteString("name")
	buf.Write([]byte{0x05, 0x00}) // string length = 5
	buf.WriteString("world")

	// TAG_End to close the compound
	buf.WriteByte(byte(tagEnd))

	// Create a TagReader with LittleEndian encoding
	reader := NewTagReader(LittleEndian)
	offsetReader := newOffsetReader(&buf)

	// Read the root tag
	tagType, tagName, err := reader.ReadTag(offsetReader)
	if err != nil {
		fmt.Printf("Error reading root tag: %v\n", err)
		return
	}

	fmt.Printf("Root tag: type=%s, name=%s\n", tagType, tagName)

	// Read the compound value
	if tagType == tagStruct {
		compound, err := reader.ReadTagCompound(offsetReader)
		if err != nil {
			fmt.Printf("Error reading compound: %v\n", err)
			return
		}

		fmt.Printf("Compound contents:\n")
		// Print in a deterministic order for the example
		if level, ok := compound["level"]; ok {
			fmt.Printf("  level: %v\n", level)
		}
		if name, ok := compound["name"]; ok {
			fmt.Printf("  name: %v\n", name)
		}
	}

	// Output:
	// Root tag: type=TAG_Compound, name=root
	// Compound contents:
	//   level: 42
	//   name: world
}

// ExampleTagReader_ReadTagValue demonstrates reading individual tag values
func ExampleTagReader_ReadTagValue() {
	reader := NewTagReader(LittleEndian)

	// Read a byte value
	byteData := []byte{0xFF}
	byteReader := newOffsetReader(bytes.NewReader(byteData))
	value, err := reader.ReadTagValue(byteReader, tagByte)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	fmt.Printf("Byte value: %d\n", value)

	// Read a string value
	// String: length (2 bytes) + "hello" (5 bytes)
	stringData := []byte{0x05, 0x00, 'h', 'e', 'l', 'l', 'o'}
	stringReader := newOffsetReader(bytes.NewReader(stringData))
	value, err = reader.ReadTagValue(stringReader, tagString)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	fmt.Printf("String value: %s\n", value)

	// Output:
	// Byte value: 255
	// String value: hello
}

// ExampleTagReader_ReadTagList demonstrates reading a TAG_List
func ExampleTagReader_ReadTagList() {
	reader := NewTagReader(LittleEndian)

	// Create a list of bytes: [1, 2, 3]
	var buf bytes.Buffer
	buf.WriteByte(byte(tagByte))              // element type: TAG_Byte
	buf.Write([]byte{0x03, 0x00, 0x00, 0x00}) // length: 3 (little endian int32)
	buf.WriteByte(1)                          // element 1
	buf.WriteByte(2)                          // element 2
	buf.WriteByte(3)                          // element 3

	offsetReader := newOffsetReader(&buf)
	list, err := reader.ReadTagList(offsetReader)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("List contents: %v\n", list)

	// Output:
	// List contents: [1 2 3]
}

// ExampleTagReader_skipMethods demonstrates how to use Skip methods to efficiently navigate NBT data
func ExampleTagReader_skipMethods() {
	reader := NewTagReader(LittleEndian)

	// Create NBT data with multiple tags where we only want to read the last one
	var buf bytes.Buffer

	// TAG_Compound (root)
	buf.WriteByte(byte(tagStruct))
	buf.Write([]byte{0x04, 0x00}) // name length = 4
	buf.WriteString("root")

	// TAG_String named "skip_me" (we'll skip this)
	buf.WriteByte(byte(tagString))
	buf.Write([]byte{0x07, 0x00}) // name length = 7
	buf.WriteString("skip_me")
	buf.Write([]byte{0x0B, 0x00}) // string length = 11
	buf.WriteString("unimportant")

	// TAG_ByteArray named "also_skip" (we'll skip this too)
	buf.WriteByte(byte(tagByteArray))
	buf.Write([]byte{0x09, 0x00}) // name length = 9
	buf.WriteString("also_skip")
	buf.Write([]byte{0x05, 0x00, 0x00, 0x00}) // array length = 5
	buf.Write([]byte{1, 2, 3, 4, 5})          // array data

	// TAG_Byte named "target" (this is what we want to read)
	buf.WriteByte(byte(tagByte))
	buf.Write([]byte{0x06, 0x00}) // name length = 6
	buf.WriteString("target")
	buf.WriteByte(42) // value = 42

	// TAG_End
	buf.WriteByte(byte(tagEnd))

	offsetReader := newOffsetReader(&buf)

	// Read root compound tag
	_, rootName, err := reader.ReadTag(offsetReader)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	fmt.Printf("Reading compound: %s\n", rootName)

	// Skip the first tag (string)
	skipType, skipName, err := reader.ReadTag(offsetReader)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	fmt.Printf("Skipping tag: %s (type: %s)\n", skipName, skipType)
	err = reader.SkipTagValue(offsetReader, skipType)
	if err != nil {
		fmt.Printf("Error skipping: %v\n", err)
		return
	}

	// Skip the second tag (byte array)
	skipType2, skipName2, err := reader.ReadTag(offsetReader)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	fmt.Printf("Skipping tag: %s (type: %s)\n", skipName2, skipType2)
	err = reader.SkipTagValue(offsetReader, skipType2)
	if err != nil {
		fmt.Printf("Error skipping: %v\n", err)
		return
	}

	// Read the target tag
	targetType, targetName, err := reader.ReadTag(offsetReader)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	fmt.Printf("Found target tag: %s (type: %s)\n", targetName, targetType)

	// Read the target value
	value, err := reader.ReadTagValue(offsetReader, targetType)
	if err != nil {
		fmt.Printf("Error reading value: %v\n", err)
		return
	}
	fmt.Printf("Target value: %v\n", value)

	// Output:
	// Reading compound: root
	// Skipping tag: skip_me (type: TAG_String)
	// Skipping tag: also_skip (type: TAG_ByteArray)
	// Found target tag: target (type: TAG_Byte)
	// Target value: 42
}

// ExampleTagReader_skipTag demonstrates skipping complete tags including their names
func ExampleTagReader_skipTag() {
	reader := NewTagReader(LittleEndian)

	// Create data with multiple complete tags
	var buf bytes.Buffer

	// First tag: TAG_String named "first"
	buf.WriteByte(byte(tagString))
	buf.Write([]byte{0x05, 0x00}) // name length = 5
	buf.WriteString("first")
	buf.Write([]byte{0x05, 0x00}) // string length = 5
	buf.WriteString("hello")

	// Second tag: TAG_Byte named "second"
	buf.WriteByte(byte(tagByte))
	buf.Write([]byte{0x06, 0x00}) // name length = 6
	buf.WriteString("second")
	buf.WriteByte(99) // value = 99

	offsetReader := newOffsetReader(&buf)

	// Skip the first complete tag
	fmt.Println("Skipping first tag...")
	err := reader.SkipTag(offsetReader)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	// Read the second tag
	tagType, tagName, err := reader.ReadTag(offsetReader)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	value, err := reader.ReadTagValue(offsetReader, tagType)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Found tag: %s = %v\n", tagName, value)

	// Output:
	// Skipping first tag...
	// Found tag: second = 99
}
