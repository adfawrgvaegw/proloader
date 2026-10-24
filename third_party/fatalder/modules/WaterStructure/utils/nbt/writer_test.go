package nbt

import (
	"bytes"
	"reflect"
	"testing"
)

func TestTagWriter_RoundTrip(t *testing.T) {
	buf := &bytes.Buffer{}
	writer := NewTagWriter(NetworkLittleEndian)
	offsetWriter := NewOffsetWriter(buf)

	data := map[string]interface{}{
		"ByteVal":      byte(0x7f),
		"ShortVal":     int16(12345),
		"IntVal":       int32(123456789),
		"LongVal":      int64(-9876543210),
		"FloatVal":     float32(1.5),
		"DoubleVal":    float64(2.25),
		"StringVal":    "hello",
		"ByteArrayVal": []byte{1, 2, 3},
		"IntArrayVal":  []int32{4, 5},
		"LongArrayVal": []int64{6, 7},
		"ListVal":      []interface{}{int32(1), int32(2), int32(3)},
		"EmptyList":    []interface{}{},
		"CompoundVal": map[string]interface{}{
			"NestedString": "nested",
			"NestedList":   []interface{}{float32(1.0)},
		},
	}

	if err := writer.WriteTag(offsetWriter, tagStruct, "Root"); err != nil {
		t.Fatalf("WriteTag returned error: %v", err)
	}
	if err := writer.WriteTagValue(offsetWriter, tagStruct, data); err != nil {
		t.Fatalf("WriteTagValue returned error: %v", err)
	}

	if offsetWriter.off != int64(buf.Len()) {
		t.Fatalf("offsetWriter tracked %d bytes, buffer has %d", offsetWriter.off, buf.Len())
	}

	offsetReader := NewOffsetReader(bytes.NewReader(buf.Bytes()))
	reader := NewTagReader(NetworkLittleEndian)

	tagType, tagName, err := reader.ReadTag(offsetReader)
	if err != nil {
		t.Fatalf("ReadTag returned error: %v", err)
	}
	if tagType != tagStruct {
		t.Fatalf("expected tag type %v, got %v", tagStruct, tagType)
	}
	if tagName != "Root" {
		t.Fatalf("expected tag name Root, got %q", tagName)
	}

	value, err := reader.ReadTagValue(offsetReader, tagType)
	if err != nil {
		t.Fatalf("ReadTagValue returned error: %v", err)
	}

	compound, ok := value.(map[string]interface{})
	if !ok {
		t.Fatalf("expected compound value, got %T", value)
	}

	if got, ok := compound["ByteVal"].(byte); !ok || got != data["ByteVal"].(byte) {
		t.Fatalf("unexpected ByteVal: %#v", compound["ByteVal"])
	}
	if got, ok := compound["ShortVal"].(int16); !ok || got != data["ShortVal"].(int16) {
		t.Fatalf("unexpected ShortVal: %#v", compound["ShortVal"])
	}
	if got, ok := compound["IntVal"].(int32); !ok || got != data["IntVal"].(int32) {
		t.Fatalf("unexpected IntVal: %#v", compound["IntVal"])
	}
	if got, ok := compound["LongVal"].(int64); !ok || got != data["LongVal"].(int64) {
		t.Fatalf("unexpected LongVal: %#v", compound["LongVal"])
	}
	if got, ok := compound["FloatVal"].(float32); !ok || got != data["FloatVal"].(float32) {
		t.Fatalf("unexpected FloatVal: %#v", compound["FloatVal"])
	}
	if got, ok := compound["DoubleVal"].(float64); !ok || got != data["DoubleVal"].(float64) {
		t.Fatalf("unexpected DoubleVal: %#v", compound["DoubleVal"])
	}
	if got, ok := compound["StringVal"].(string); !ok || got != data["StringVal"].(string) {
		t.Fatalf("unexpected StringVal: %#v", compound["StringVal"])
	}

	if got, ok := compound["ByteArrayVal"].([]byte); !ok || !reflect.DeepEqual(got, data["ByteArrayVal"].([]byte)) {
		t.Fatalf("unexpected ByteArrayVal: %#v", compound["ByteArrayVal"])
	}
	if got, ok := compound["IntArrayVal"].([]int32); !ok || !reflect.DeepEqual(got, data["IntArrayVal"].([]int32)) {
		t.Fatalf("unexpected IntArrayVal: %#v", compound["IntArrayVal"])
	}
	if got, ok := compound["LongArrayVal"].([]int64); !ok || !reflect.DeepEqual(got, data["LongArrayVal"].([]int64)) {
		t.Fatalf("unexpected LongArrayVal: %#v", compound["LongArrayVal"])
	}

	if got, ok := compound["ListVal"].([]interface{}); !ok || !reflect.DeepEqual(got, data["ListVal"].([]interface{})) {
		t.Fatalf("unexpected ListVal: %#v", compound["ListVal"])
	}
	if got, ok := compound["EmptyList"].([]interface{}); !ok || len(got) != 0 {
		t.Fatalf("unexpected EmptyList: %#v", compound["EmptyList"])
	}

	nestedRaw, ok := compound["CompoundVal"].(map[string]interface{})
	if !ok {
		t.Fatalf("unexpected CompoundVal: %#v", compound["CompoundVal"])
	}
	if nested, ok := nestedRaw["NestedString"].(string); !ok || nested != "nested" {
		t.Fatalf("unexpected NestedString: %#v", nestedRaw["NestedString"])
	}
	nestedList, ok := nestedRaw["NestedList"].([]interface{})
	if !ok || len(nestedList) != 1 {
		t.Fatalf("unexpected NestedList: %#v", nestedRaw["NestedList"])
	}
	if got, ok := nestedList[0].(float32); !ok || got != float32(1.0) {
		t.Fatalf("unexpected NestedList element: %#v", nestedList[0])
	}
}

func TestTagWriter_WriteTagType_Invalid(t *testing.T) {
	buf := &bytes.Buffer{}
	writer := NewTagWriter(NetworkLittleEndian)
	offsetWriter := NewOffsetWriter(buf)

	if err := writer.WriteTagType(offsetWriter, tagType(0xFF)); err == nil {
		t.Fatalf("expected error for unknown tag type")
	} else if _, ok := err.(UnknownTagError); !ok {
		t.Fatalf("expected UnknownTagError, got %T", err)
	}
}

func TestTagWriter_WriteTagListMixedTypes(t *testing.T) {
	buf := &bytes.Buffer{}
	writer := NewTagWriter(NetworkLittleEndian)
	offsetWriter := NewOffsetWriter(buf)

	if err := writer.WriteTagList(offsetWriter, []interface{}{int32(1), "bad"}); err == nil {
		t.Fatalf("expected error for mixed list types")
	} else if _, ok := err.(IncompatibleTypeError); !ok {
		t.Fatalf("expected IncompatibleTypeError, got %T", err)
	}
}
