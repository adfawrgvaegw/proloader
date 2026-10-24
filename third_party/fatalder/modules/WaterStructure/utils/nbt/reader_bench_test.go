package nbt

import (
	"bytes"
	"testing"
)

// BenchmarkSkipLargeByteArray_Optimized benchmarks the optimized skip method using io.CopyN
func BenchmarkSkipLargeByteArray_Optimized(b *testing.B) {
	// Create a 1MB byte array
	largeSize := int32(1024 * 1024)
	var buf bytes.Buffer

	// Write array length
	buf.Write([]byte{
		byte(largeSize), byte(largeSize >> 8), byte(largeSize >> 16), byte(largeSize >> 24),
	})

	// Write large data
	for i := int32(0); i < largeSize; i++ {
		buf.WriteByte(0x00)
	}

	data := buf.Bytes()
	reader := NewTagReader(LittleEndian)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		offsetReader := newOffsetReader(bytes.NewReader(data))
		err := reader.SkipTagByteArray(offsetReader)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkSkipLargeByteArray_Naive benchmarks a naive implementation that allocates memory
func BenchmarkSkipLargeByteArray_Naive(b *testing.B) {
	// Create a 1MB byte array
	largeSize := int32(1024 * 1024)
	var buf bytes.Buffer

	// Write array length
	buf.Write([]byte{
		byte(largeSize), byte(largeSize >> 8), byte(largeSize >> 16), byte(largeSize >> 24),
	})

	// Write large data
	for i := int32(0); i < largeSize; i++ {
		buf.WriteByte(0x00)
	}

	data := buf.Bytes()
	reader := NewTagReader(LittleEndian)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		offsetReader := newOffsetReader(bytes.NewReader(data))

		// Naive implementation: allocate memory and read into it
		length, err := reader.endian.Int32(offsetReader)
		if err != nil {
			b.Fatal(err)
		}
		if length < 0 {
			b.Fatal("negative length")
		}

		// This allocates 1MB of memory each time
		buffer := make([]byte, length)
		_, err = offsetReader.Read(buffer)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkSkipLargeString_Optimized benchmarks the optimized string skip method
func BenchmarkSkipLargeString_Optimized(b *testing.B) {
	// Create a 16KB string
	largeSize := int16(16384)
	var buf bytes.Buffer

	// Write string length (little endian int16)
	buf.Write([]byte{byte(largeSize), byte(largeSize >> 8)})

	// Write large string data
	for i := int16(0); i < largeSize; i++ {
		buf.WriteByte('A')
	}

	data := buf.Bytes()
	reader := NewTagReader(LittleEndian)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		offsetReader := newOffsetReader(bytes.NewReader(data))
		err := reader.SkipTagString(offsetReader)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkSkipLargeString_Naive benchmarks a naive string skip implementation
func BenchmarkSkipLargeString_Naive(b *testing.B) {
	// Create a 16KB string
	largeSize := int16(16384)
	var buf bytes.Buffer

	// Write string length (little endian int16)
	buf.Write([]byte{byte(largeSize), byte(largeSize >> 8)})

	// Write large string data
	for i := int16(0); i < largeSize; i++ {
		buf.WriteByte('A')
	}

	data := buf.Bytes()
	reader := NewTagReader(LittleEndian)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		offsetReader := newOffsetReader(bytes.NewReader(data))

		// Naive implementation: allocate memory and read into it
		length, err := reader.endian.Int16(offsetReader)
		if err != nil {
			b.Fatal(err)
		}
		if length < 0 {
			b.Fatal("negative length")
		}

		// This allocates 16KB of memory each time
		buffer := make([]byte, length)
		_, err = offsetReader.Read(buffer)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkSkipSmallData shows that for small data, the difference is minimal
func BenchmarkSkipSmallData_Optimized(b *testing.B) {
	// Create a small 64-byte array
	smallSize := int32(64)
	var buf bytes.Buffer

	// Write array length
	buf.Write([]byte{
		byte(smallSize), byte(smallSize >> 8), byte(smallSize >> 16), byte(smallSize >> 24),
	})

	// Write small data
	for i := int32(0); i < smallSize; i++ {
		buf.WriteByte(0x00)
	}

	data := buf.Bytes()
	reader := NewTagReader(LittleEndian)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		offsetReader := newOffsetReader(bytes.NewReader(data))
		err := reader.SkipTagByteArray(offsetReader)
		if err != nil {
			b.Fatal(err)
		}
	}
}
