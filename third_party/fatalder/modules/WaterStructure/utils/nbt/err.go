package nbt

import (
	"errors"
	"fmt"
	"reflect"
)

// InvalidTypeError is returned when the type of a tag read is not equal to the struct field with the name
// of that tag.
type InvalidTypeError struct {
	Off       int64
	Field     string
	TagType   tagType
	FieldType reflect.Type
}

// Error ...
func (err InvalidTypeError) Error() string {
	return fmt.Sprintf("nbt: 在偏移 %v 的标签 '%v' 类型无效: 无法将 %v 反序列化为 %v", err.Off, err.Field, err.TagType, err.FieldType)
}

// UnknownTagError is returned when the type of tag read is not known, meaning it is not found in the tag.go
// file.
type UnknownTagError struct {
	Off     int64
	Op      string
	TagType tagType
}

// Error ...
func (err UnknownTagError) Error() string {
	return fmt.Sprintf("nbt: 在偏移 %v 处的操作 '%v' 发现未知标签 '%v'", err.Off, err.Op, byte(err.TagType))
}

// UnexpectedTagError is returned when a tag type encountered was not expected, and thus valid in its context.
type UnexpectedTagError struct {
	Off     int64
	TagType tagType
}

// Error ...
func (err UnexpectedTagError) Error() string {
	return fmt.Sprintf("nbt: 在偏移 %v 处遇到意外标签 %v: 该标签在当前上下文无效", err.Off, err.TagType)
}

// NonPointerTypeError is returned when the type of value passed in Decoder.Decode or Unmarshal is not a
// pointer.
type NonPointerTypeError struct {
	ActualType reflect.Type
}

// Error ...
func (err NonPointerTypeError) Error() string {
	return fmt.Sprintf("nbt: 期望指针类型用于解码, 但得到 '%v'", err.ActualType)
}

// BufferOverrunError is returned when the data buffer passed in when reading is overrun, meaning one of the
// reading operations extended beyond the end of the slice.
type BufferOverrunError struct {
	Op string
}

// Error ...
func (err BufferOverrunError) Error() string {
	return fmt.Sprintf("nbt: 操作 '%v' 期间缓冲区意外结束", err.Op)
}

// InvalidArraySizeError is returned when an array read from the NBT (that includes byte arrays, int32 arrays
// and int64 arrays) does not have the same size as the Go representation.
type InvalidArraySizeError struct {
	Off       int64
	Op        string
	GoLength  int
	NBTLength int
}

// Error ...
func (err InvalidArraySizeError) Error() string {
	return fmt.Sprintf("nbt: 在偏移 %v 的操作 '%v' 处数组大小不匹配: 期望 %v, 实际 %v", err.Off, err.Op, err.GoLength, err.NBTLength)
}

// UnexpectedNamedTagError is returned when a named tag is read from a compound which is not present in the
// struct it is decoded into.
type UnexpectedNamedTagError struct {
	Off     int64
	TagName string
	TagType tagType
}

// Error ...
func (err UnexpectedNamedTagError) Error() string {
	return fmt.Sprintf("nbt: 在偏移 %v 处遇到意外命名标签 '%v' 类型为 %v: 目标结构中不存在", err.Off, err.TagName, err.TagType)
}

// FailedWriteError is returned if a Write operation failed on an offsetWriter, meaning some of the data could
// not be written to the io.Writer.
type FailedWriteError struct {
	Off int64
	Op  string
	Err error
}

// Error ...
func (err FailedWriteError) Error() string {
	return fmt.Sprintf("nbt: 在偏移 %v 的操作 '%v' 写入失败: %v", err.Off, err.Op, err.Err)
}

// IncompatibleTypeError is returned if a value is attempted to be written to an io.Writer, but its type can-
// not be translated to an NBT tag.
type IncompatibleTypeError struct {
	ValueName string
	Type      reflect.Type
}

// Error ...
func (err IncompatibleTypeError) Error() string {
	return fmt.Sprintf("nbt: 值类型 %v（%v）无法转换为 NBT 标签", err.Type, err.ValueName)
}

var errStringTooLong = errors.New("字符串长度超过最大限制")

// InvalidStringError is returned if a string read is not valid, meaning it does not exist exclusively out of
// utf8 characters, or if it is longer than the length prefix can carry.
type InvalidStringError struct {
	Off int64
	Err error
	N   uint
}

// Error ...
func (err InvalidStringError) Error() string {
	return fmt.Sprintf("nbt: 偏移 %v 的字符串无效: %v（长度=%v）", err.Off, err.Err, err.N)
}

const maximumNestingDepth = 512

// MaximumDepthReachedError is returned if the maximum depth of 512 compound/list tags has been reached while
// reading or writing NBT.
type MaximumDepthReachedError struct {
}

// Error ...
func (err MaximumDepthReachedError) Error() string {
	return fmt.Sprintf("nbt: 达到最大嵌套深度 %v", maximumNestingDepth)
}

const maximumNetworkOffset = 4 * 1024 * 1024

// MaximumBytesReadError is returned if the maximum amount of bytes has been read for NetworkLittleEndian
// format. It is returned if the offset hits maximumNetworkOffset.
type MaximumBytesReadError struct {
}

// Error ...
func (err MaximumBytesReadError) Error() string {
	return fmt.Sprintf("nbt: NetworkLittleEndian 格式读取字节数限制 %v 已耗尽", maximumNetworkOffset)
}

// InvalidVarintError is returned if a varint(32/64) is encountered that does
// not end after 5 or 10 bytes respectively.
type InvalidVarintError struct {
	Off int64
	N   int
}

// Error ...
func (err InvalidVarintError) Error() string {
	return fmt.Sprintf("nbt: varint 在读取 %v 字节后未终止, 偏移 %v", err.N, err.Off)
}
