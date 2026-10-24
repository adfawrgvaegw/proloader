package nbt

import (
	"fmt"
	"io"
	"math"
	"reflect"
)

// offsetWriter is a wrapper around an io.Writer which keeps track of the amount of bytes written, so that it
// may be used in errors.
type offsetWriter struct {
	io.Writer
	off int64

	// WriteByte is a function implemented by offsetWriter if the io.Writer does not implement it itself.
	WriteByte func(byte) error
}

// Write writes a byte slice to the underlying io.Writer. It increases the byte offset by exactly n.
func (w *offsetWriter) Write(b []byte) (n int, err error) {
	n, err = w.Writer.Write(b)
	w.off += int64(n)
	return
}

func NewOffsetWriter(w io.Writer) *offsetWriter {
	writer := &offsetWriter{Writer: w}
	if byteWriter, ok := w.(io.ByteWriter); ok {
		writer.WriteByte = func(b byte) error {
			if err := byteWriter.WriteByte(b); err != nil {
				return err
			}
			writer.off++
			return nil
		}
	} else {
		writer.WriteByte = func(b byte) error {
			n, err := writer.Write([]byte{b})
			if err != nil {
				return err
			}
			if n != 1 {
				return io.ErrShortWrite
			}
			return nil
		}
	}
	return writer
}

func (w *offsetWriter) GetOffset() int64 {
	return w.off
}

type TagWriter struct {
	endian Encoding
	depth  int
}

func NewTagWriter(endian Encoding) *TagWriter {
	return &TagWriter{endian: endian}
}

func (w *TagWriter) WriteTagType(writer *offsetWriter, t tagType) error {
	if !t.IsValid() {
		return UnknownTagError{Off: writer.off, TagType: t, Op: "WriteTagType"}
	}
	return writeSingleByte(writer, byte(t), "WriteTagType")
}

func (w *TagWriter) WriteTagByte(writer *offsetWriter, value byte) error {
	return writeSingleByte(writer, value, "WriteTagByte")
}

func (w *TagWriter) WriteTagInt16(writer *offsetWriter, value int16) error {
	return w.endian.WriteInt16(writer, value)
}

func (w *TagWriter) WriteTagInt32(writer *offsetWriter, value int32) error {
	return w.endian.WriteInt32(writer, value)
}

func (w *TagWriter) WriteTagInt64(writer *offsetWriter, value int64) error {
	return w.endian.WriteInt64(writer, value)
}

func (w *TagWriter) WriteTagFloat32(writer *offsetWriter, value float32) error {
	return w.endian.WriteFloat32(writer, value)
}

func (w *TagWriter) WriteTagFloat64(writer *offsetWriter, value float64) error {
	return w.endian.WriteFloat64(writer, value)
}

func (w *TagWriter) WriteTagString(writer *offsetWriter, value string) error {
	return w.endian.WriteString(writer, value)
}

func (w *TagWriter) WriteTagByteArray(writer *offsetWriter, data []byte) error {
    if len(data) > math.MaxInt32 {
        return FailedWriteError{Op: "WriteTagByteArray", Off: writer.off, Err: fmt.Errorf("字节数组长度 %d 超过最大值 %d", len(data), math.MaxInt32)}
    }
	if err := w.endian.WriteInt32(writer, int32(len(data))); err != nil {
		return err
	}
	if len(data) == 0 {
		return nil
	}
	n, err := writer.Write(data)
	if err != nil {
		return FailedWriteError{Op: "WriteTagByteArray", Off: writer.off, Err: err}
	}
	if n != len(data) {
		return FailedWriteError{Op: "WriteTagByteArray", Off: writer.off, Err: io.ErrShortWrite}
	}
	return nil
}

func (w *TagWriter) WriteTagInt32Array(writer *offsetWriter, data []int32) error {
    if len(data) > math.MaxInt32 {
        return FailedWriteError{Op: "WriteTagInt32Array", Off: writer.off, Err: fmt.Errorf("int32 数组长度 %d 超过最大值 %d", len(data), math.MaxInt32)}
    }
	if err := w.endian.WriteInt32(writer, int32(len(data))); err != nil {
		return err
	}
	for _, v := range data {
		if err := w.endian.WriteInt32(writer, v); err != nil {
			return err
		}
	}
	return nil
}

func (w *TagWriter) WriteTagInt64Array(writer *offsetWriter, data []int64) error {
    if len(data) > math.MaxInt32 {
        return FailedWriteError{Op: "WriteTagInt64Array", Off: writer.off, Err: fmt.Errorf("int64 数组长度 %d 超过最大值 %d", len(data), math.MaxInt32)}
    }
	if err := w.endian.WriteInt32(writer, int32(len(data))); err != nil {
		return err
	}
	for _, v := range data {
		if err := w.endian.WriteInt64(writer, v); err != nil {
			return err
		}
	}
	return nil
}

func (w *TagWriter) WriteTag(writer *offsetWriter, t tagType, name string) error {
	if err := w.WriteTagType(writer, t); err != nil {
		return err
	}
	if t == tagEnd {
		return nil
	}
	if _, ok := w.endian.(networkBigEndian); ok && t == tagStruct && w.depth == 0 {
		return nil
	}
	return w.WriteTagString(writer, name)
}

func (w *TagWriter) WriteTagValue(writer *offsetWriter, t tagType, value interface{}) error {
	return w.writeTagValue(writer, t, value, "")
}

func (w *TagWriter) WriteTagList(writer *offsetWriter, list []interface{}) error {
	return w.writeTagList(writer, list, "")
}

func (w *TagWriter) WriteTagCompound(writer *offsetWriter, compound map[string]interface{}) error {
	return w.writeTagCompound(writer, compound, "")
}

func (w *TagWriter) writeTagValue(writer *offsetWriter, t tagType, value interface{}, valueName string) error {
	switch t {
	case tagEnd:
		return nil
	case tagByte:
		v, err := w.asByte(value, valueName)
		if err != nil {
			return err
		}
		return w.WriteTagByte(writer, v)
	case tagInt16:
		if v, ok := value.(int16); ok {
			return w.WriteTagInt16(writer, v)
		}
		return IncompatibleTypeError{Type: reflect.TypeOf(value), ValueName: valueName}
	case tagInt32:
		if v, ok := value.(int32); ok {
			return w.WriteTagInt32(writer, v)
		}
		return IncompatibleTypeError{Type: reflect.TypeOf(value), ValueName: valueName}
	case tagInt64:
		if v, ok := value.(int64); ok {
			return w.WriteTagInt64(writer, v)
		}
		return IncompatibleTypeError{Type: reflect.TypeOf(value), ValueName: valueName}
	case tagFloat32:
		if v, ok := value.(float32); ok {
			return w.WriteTagFloat32(writer, v)
		}
		return IncompatibleTypeError{Type: reflect.TypeOf(value), ValueName: valueName}
	case tagFloat64:
		if v, ok := value.(float64); ok {
			return w.WriteTagFloat64(writer, v)
		}
		return IncompatibleTypeError{Type: reflect.TypeOf(value), ValueName: valueName}
	case tagByteArray:
		data, err := w.asByteArray(value, valueName)
		if err != nil {
			return err
		}
		return w.WriteTagByteArray(writer, data)
	case tagString:
		if v, ok := value.(string); ok {
			return w.WriteTagString(writer, v)
		}
		return IncompatibleTypeError{Type: reflect.TypeOf(value), ValueName: valueName}
	case tagInt32Array:
		data, err := w.asInt32Array(value, valueName)
		if err != nil {
			return err
		}
		return w.WriteTagInt32Array(writer, data)
	case tagInt64Array:
		data, err := w.asInt64Array(value, valueName)
		if err != nil {
			return err
		}
		return w.WriteTagInt64Array(writer, data)
	case tagSlice:
		list, err := w.asInterfaceSlice(value, valueName)
		if err != nil {
			return err
		}
		return w.writeTagList(writer, list, valueName)
	case tagStruct:
		compound, err := w.asCompoundMap(value, valueName)
		if err != nil {
			return err
		}
		return w.writeTagCompound(writer, compound, valueName)
	default:
		return UnknownTagError{Off: writer.off, TagType: t, Op: "WriteTagValue"}
	}
}

func (w *TagWriter) writeTagList(writer *offsetWriter, list []interface{}, listName string) error {
	if err := w.enter(); err != nil {
		return err
	}
	defer w.leave()

	elementType := tagEnd
	if len(list) > 0 {
		var err error
		elementType, err = w.inferTagType(list[0], listElementName(listName, 0))
		if err != nil {
			return err
		}
		if elementType == tagEnd {
			return IncompatibleTypeError{Type: reflect.TypeOf(list[0]), ValueName: listElementName(listName, 0)}
		}
	}
	if err := w.WriteTagType(writer, elementType); err != nil {
		return err
	}
	if err := w.endian.WriteInt32(writer, int32(len(list))); err != nil {
		return err
	}
	for i, elem := range list {
		elemName := listElementName(listName, i)
		t, err := w.inferTagType(elem, elemName)
		if err != nil {
			return err
		}
		if t != elementType {
			return IncompatibleTypeError{Type: reflect.TypeOf(elem), ValueName: elemName}
		}
		if err := w.writeTagValue(writer, elementType, elem, elemName); err != nil {
			return err
		}
	}
	return nil
}

func (w *TagWriter) writeTagCompound(writer *offsetWriter, compound map[string]interface{}, compoundName string) error {
	if err := w.enter(); err != nil {
		return err
	}
	defer w.leave()

	for name, value := range compound {
		entryName := name
		if compoundName != "" {
			entryName = compoundName + "." + name
		}
		tagType, err := w.inferTagType(value, entryName)
		if err != nil {
			return err
		}
		if err := w.WriteTag(writer, tagType, name); err != nil {
			return err
		}
		if err := w.writeTagValue(writer, tagType, value, entryName); err != nil {
			return err
		}
	}

	return writeSingleByte(writer, byte(tagEnd), "WriteTagCompoundEnd")
}

func (w *TagWriter) enter() error {
	if w.depth >= maximumNestingDepth {
		return MaximumDepthReachedError{}
	}
	w.depth++
	return nil
}

func (w *TagWriter) leave() {
	if w.depth > 0 {
		w.depth--
	}
}

func (w *TagWriter) inferTagType(value interface{}, valueName string) (tagType, error) {
	if value == nil {
		return 0, IncompatibleTypeError{Type: nil, ValueName: valueName}
	}
	switch value.(type) {
	case uint8, bool:
		return tagByte, nil
	case int16:
		return tagInt16, nil
	case int32:
		return tagInt32, nil
	case int64:
		return tagInt64, nil
	case float32:
		return tagFloat32, nil
	case float64:
		return tagFloat64, nil
	case string:
		return tagString, nil
	case []byte:
		return tagByteArray, nil
	case []int32:
		return tagInt32Array, nil
	case []int64:
		return tagInt64Array, nil
	case []interface{}:
		return tagSlice, nil
	case map[string]interface{}:
		return tagStruct, nil
	}

	val := reflect.ValueOf(value)
	if !val.IsValid() {
		return 0, IncompatibleTypeError{Type: nil, ValueName: valueName}
	}

	switch val.Kind() {
	case reflect.Map:
		if val.Type().Key().Kind() == reflect.String {
			return tagStruct, nil
		}
	case reflect.Slice:
		switch val.Type().Elem().Kind() {
		case reflect.Uint8:
			return tagByteArray, nil
		case reflect.Int32:
			return tagInt32Array, nil
		case reflect.Int64:
			return tagInt64Array, nil
		default:
			return tagSlice, nil
		}
	case reflect.Array:
		switch val.Type().Elem().Kind() {
		case reflect.Uint8:
			return tagByteArray, nil
		case reflect.Int32:
			return tagInt32Array, nil
		case reflect.Int64:
			return tagInt64Array, nil
		default:
			return tagSlice, nil
		}
	}

	return 0, IncompatibleTypeError{Type: val.Type(), ValueName: valueName}
}

func (w *TagWriter) asByte(value interface{}, valueName string) (byte, error) {
	switch v := value.(type) {
	case uint8:
		return byte(v), nil
	case bool:
		if v {
			return 1, nil
		}
		return 0, nil
	}
	return 0, IncompatibleTypeError{Type: reflect.TypeOf(value), ValueName: valueName}
}

func (w *TagWriter) asByteArray(value interface{}, valueName string) ([]byte, error) {
	if value == nil {
		return nil, IncompatibleTypeError{Type: nil, ValueName: valueName}
	}
	if data, ok := value.([]byte); ok {
		return data, nil
	}
	val := reflect.ValueOf(value)
	if !val.IsValid() {
		return nil, IncompatibleTypeError{Type: nil, ValueName: valueName}
	}
	if val.Kind() == reflect.Array && val.Type().Elem().Kind() == reflect.Uint8 {
		data := make([]byte, val.Len())
		for i := 0; i < val.Len(); i++ {
			data[i] = byte(val.Index(i).Uint())
		}
		return data, nil
	}
	return nil, IncompatibleTypeError{Type: val.Type(), ValueName: valueName}
}

func (w *TagWriter) asInt32Array(value interface{}, valueName string) ([]int32, error) {
	if value == nil {
		return nil, IncompatibleTypeError{Type: nil, ValueName: valueName}
	}
	if data, ok := value.([]int32); ok {
		return data, nil
	}
	val := reflect.ValueOf(value)
	if !val.IsValid() {
		return nil, IncompatibleTypeError{Type: nil, ValueName: valueName}
	}
	if val.Kind() == reflect.Array && val.Type().Elem().Kind() == reflect.Int32 {
		data := make([]int32, val.Len())
		for i := 0; i < val.Len(); i++ {
			data[i] = int32(val.Index(i).Int())
		}
		return data, nil
	}
	return nil, IncompatibleTypeError{Type: val.Type(), ValueName: valueName}
}

func (w *TagWriter) asInt64Array(value interface{}, valueName string) ([]int64, error) {
	if value == nil {
		return nil, IncompatibleTypeError{Type: nil, ValueName: valueName}
	}
	if data, ok := value.([]int64); ok {
		return data, nil
	}
	val := reflect.ValueOf(value)
	if !val.IsValid() {
		return nil, IncompatibleTypeError{Type: nil, ValueName: valueName}
	}
	if val.Kind() == reflect.Array && val.Type().Elem().Kind() == reflect.Int64 {
		data := make([]int64, val.Len())
		for i := 0; i < val.Len(); i++ {
			data[i] = val.Index(i).Int()
		}
		return data, nil
	}
	return nil, IncompatibleTypeError{Type: val.Type(), ValueName: valueName}
}

func (w *TagWriter) asInterfaceSlice(value interface{}, valueName string) ([]interface{}, error) {
	if value == nil {
		return []interface{}{}, nil
	}
	if list, ok := value.([]interface{}); ok {
		return list, nil
	}
	val := reflect.ValueOf(value)
	if !val.IsValid() {
		return nil, IncompatibleTypeError{Type: nil, ValueName: valueName}
	}
	if val.Kind() == reflect.Slice || val.Kind() == reflect.Array {
		length := val.Len()
		result := make([]interface{}, length)
		for i := 0; i < length; i++ {
			result[i] = val.Index(i).Interface()
		}
		return result, nil
	}
	return nil, IncompatibleTypeError{Type: val.Type(), ValueName: valueName}
}

func (w *TagWriter) asCompoundMap(value interface{}, valueName string) (map[string]interface{}, error) {
	if value == nil {
		return map[string]interface{}{}, nil
	}
	if compound, ok := value.(map[string]interface{}); ok {
		return compound, nil
	}
	val := reflect.ValueOf(value)
	if !val.IsValid() {
		return nil, IncompatibleTypeError{Type: nil, ValueName: valueName}
	}
	if val.Kind() == reflect.Map && val.Type().Key().Kind() == reflect.String {
		if val.IsNil() {
			return map[string]interface{}{}, nil
		}
		result := make(map[string]interface{}, val.Len())
		iter := val.MapRange()
		for iter.Next() {
			result[iter.Key().String()] = iter.Value().Interface()
		}
		return result, nil
	}
	return nil, IncompatibleTypeError{Type: val.Type(), ValueName: valueName}
}

func listElementName(base string, index int) string {
    if base == "" {
        return fmt.Sprintf("列表[%d]", index)
    }
    return fmt.Sprintf("%s[%d]", base, index)
}

func writeSingleByte(writer *offsetWriter, b byte, op string) error {
	if err := writer.WriteByte(b); err != nil {
		return FailedWriteError{Op: op, Off: writer.off, Err: err}
	}
	return nil
}
