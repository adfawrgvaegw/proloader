package pymarshal

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
)

var ErrNull = errors.New("null object")

var PycHeader = []byte{0x3, 0xF3, 0xD, 0xA, 0, 0, 0, 0}

func PycRepairer(data []byte, strip bool) ([]byte, error) {
	var result bytes.Buffer

	if !strip {
		depth = 1000
	} else {
		depth = 0
	}

	strings = make([][]byte, 0)

	result.Write(PycHeader)

	readBuffer := bytes.NewReader(data)

	err := RepairMarshal(readBuffer, &result)
	if err != nil {
		return nil, err
	}
	//println(readBuffer.Len(), "/", readBuffer.Size())
	// err = RepairMarshal(readBuffer, &result)

	return result.Bytes(), err
}

var strings = make([][]byte, 0)
var depth = 0

func RepairMarshal(reader *bytes.Reader, writer *bytes.Buffer) error {
	tagType, err := reader.ReadByte()
	if err != nil {
		return err
	}

	skip := true
	switch tagType {
	case TYPE_NETEASE_STRING_0:
		//println("type: netease string v0")
		writer.WriteByte(TYPE_STRING)
		err = repairNetEaseString0(reader, writer, true)
	case TYPE_NETEASE_UNICODE_0:
		//println("type: netease unicode v0")
		writer.WriteByte(TYPE_UNICODE)
		err = repairNetEaseString0(reader, writer, true)
	case TYPE_NETEASE_INTERNED_0:
		//println("type: netease interned v0")
		writer.WriteByte(TYPE_STRING)
		err = repairNetEaseString0(reader, writer, false)
	case TYPE_NETEASE_INTERNED_1:
		//println("type: netease interned v1")
		writer.WriteByte(TYPE_STRING)
		err = repairNetEaseString1(reader, writer, false)
	case TYPE_NETEASE_UNICODE_1:
		//println("type: netease unicode v1")
		writer.WriteByte(TYPE_UNICODE)
		err = repairNetEaseString1(reader, writer, true)
	case TYPE_NETEASE_STRING_1:
		//println("type: netease string v1")
		writer.WriteByte(TYPE_STRING)
		err = repairNetEaseString1(reader, writer, true)
	case TYPE_NETEASE_INTERNED_2:
		//println("type: netease interned v2")
		writer.WriteByte(TYPE_STRING)
		err = repairNetEaseString2(reader, writer, false)
	case TYPE_NETEASE_UNICODE_2:
		//println("type: netease unicode v2")
		writer.WriteByte(TYPE_UNICODE)
		err = repairNetEaseString2(reader, writer, true)
	case TYPE_NETEASE_STRING_2:
		//println("type: netease string v2")
		writer.WriteByte(TYPE_STRING)
		err = repairNetEaseString2(reader, writer, true)
	case TYPE_NETEASE_CODE_0:
		depth++
		//println("type: netease code v0")
		writer.WriteByte(TYPE_CODE)
		err = repairNetEaseCode0(reader, writer)
		depth--
	case TYPE_NETEASE_CODE_1:
		depth++
		//println("type: netease code v1")
		writer.WriteByte(TYPE_CODE)
		err = repairNetEaseCode1(reader, writer)
		depth--
	case TYPE_NETEASE_CODE_2:
		depth++
		//println("type: netease code v2")
		writer.WriteByte(TYPE_CODE)
		err = repairNetEaseCode2(reader, writer)
		depth--
	case TYPE_INTERNED:
		//println("type: string interned")
		writer.WriteByte(TYPE_STRING)
		err = copyStringInterned(reader, writer)
	case TYPE_STRINGREF:
		writer.WriteByte(TYPE_STRING)
		var n int32
		err := binary.Read(reader, binary.LittleEndian, &n)
		if err != nil {
			return err
		}

		str := strings[n]
		err = binary.Write(writer, binary.LittleEndian, int32(len(str)))
		if err != nil {
			return err
		}
		writer.Write(str)
	default:
		skip = false
	}
	if err != nil {
		return err
	} else if skip {
		return nil
	}

	writer.WriteByte(tagType)

	switch tagType {
	// theres no extra data for these types below
	// i just print out the exact type for debug purpose
	case TYPE_NULL:
		//println("type: null")
		// err = ErrNull
	case TYPE_NONE:
		//println("type: none")
		// err = ErrNull
	case TYPE_STOPITER:
		//println("type: stopiter")
	case TYPE_ELLIPSIS:
		//println("type: ellipsis")
	case TYPE_FALSE:
		//println("type: bool false")
	case TYPE_TRUE:
		//println("type: bool true")
	// these types below contrails extra data
	case TYPE_INT:
		//println("type: int32")
		err = copyBytes(reader, writer, 4)
	case TYPE_INT64:
		//println("type: int64")
		err = copyBytes(reader, writer, 8)
	case TYPE_LONG:
		//println("type: long")
		err = copyPyLong(reader, writer)
	case TYPE_FLOAT:
		//println("type: float")
		err = copyFloat(reader, writer)
	case TYPE_BINARY_FLOAT:
		//println("type: binary float")
		err = copyBytes(reader, writer, 8)
	case TYPE_COMPLEX:
		//println("type: complex")
		err = copyFloat(reader, writer)
		if err != nil {
			return err
		}
		err = copyFloat(reader, writer)
	case TYPE_BINARY_COMPLEX:
		//println("type: binary complex")
		err = copyBytes(reader, writer, 16)
	case TYPE_UNICODE:
		fallthrough
	case TYPE_STRING:
		//println("type: string")
		err = copyString(reader, writer)
	// case TYPE_STRINGREF:
	// 	println("type: string reference")
	// 	err = copyBytes(reader, writer, 4)
	// reader.Read([]byte{0x00, 0x00, 0x00, 0x00})
	// writer.Write([]byte{0x00, 0x00, 0x00, 0x00})
	case TYPE_TUPLE:
		fallthrough
	case TYPE_SET:
		fallthrough
	case TYPE_FROZENSET:
		fallthrough
	case TYPE_LIST:
		//println("type: list-liked", tagType)
		err = copyList(reader, writer)
	case TYPE_DICT:
		//println("type: dict")
		err = copyDict(reader, writer)
	case TYPE_CODE:
		//println("type: code")
		err = copyBytes(reader, writer, 4*4)
		if err != nil {
			return err
		}
		err = repairOpcode(reader, writer, 0)
		if err != nil {
			return err
		}
		for i := 0; i < 7; i++ {
			err = RepairMarshal(reader, writer)
			if err != nil {
				return err
			}
		}
		err = copyBytes(reader, writer, 4)
		if err != nil {
			return err
		}
		err = RepairMarshal(reader, writer)
	default:
		err = fmt.Errorf("unknown type: %x", tagType)
	}
	return err
}

func copyBytes(reader *bytes.Reader, writer *bytes.Buffer, size int32) error {
	if size == 0 {
		return nil
	}

	data := make([]byte, size)

	_, err := reader.Read(data)
	if err != nil {
		return err
	}

	_, err = writer.Write(data)
	return err
}

func copyFloat(reader *bytes.Reader, writer *bytes.Buffer) error {
	size, err := reader.ReadByte()
	if err != nil {
		return err
	}
	err = writer.WriteByte(size)
	if err != nil {
		return err
	}

	return copyBytes(reader, writer, int32(size))
}

func copyString(reader *bytes.Reader, writer *bytes.Buffer) error {
	var n int32
	err := binary.Read(reader, binary.LittleEndian, &n)
	if err != nil {
		return err
	}

	err = binary.Write(writer, binary.LittleEndian, n)
	if err != nil {
		return err
	}

	return copyBytes(reader, writer, n)
}

func copyStringInterned(reader *bytes.Reader, writer *bytes.Buffer) error {
	var n int32
	err := binary.Read(reader, binary.LittleEndian, &n)
	if err != nil {
		return err
	}

	err = binary.Write(writer, binary.LittleEndian, n)
	if err != nil {
		return err
	}

	data := make([]byte, n)

	_, err = reader.Read(data)
	if err != nil {
		return err
	}

	_, err = writer.Write(data)
	if err != nil {
		return err
	}

	strings = append(strings, data)

	return nil
}

func copyList(reader *bytes.Reader, writer *bytes.Buffer) error {
	var n int32
	err := binary.Read(reader, binary.LittleEndian, &n)
	if err != nil {
		return err
	}

	err = binary.Write(writer, binary.LittleEndian, n)
	if err != nil {
		return err
	}

	for i := int32(0); i < n; i++ {
		err = RepairMarshal(reader, writer)
		if err != nil && !errors.Is(err, ErrNull) {
			return err
		}
	}

	return nil
}

func copyDict(reader *bytes.Reader, writer *bytes.Buffer) error {
	// read until key is null object
	for {
		err := RepairMarshal(reader, writer)
		if errors.Is(err, ErrNull) {
			break
		} else if err != nil {
			return err
		}
		err = RepairMarshal(reader, writer)
		if err != nil && !errors.Is(err, ErrNull) {
			return err
		}
	}
	return nil
}

func copyPyLong(reader *bytes.Reader, writer *bytes.Buffer) error {
	var n int32
	err := binary.Read(reader, binary.LittleEndian, &n)
	if err != nil {
		return err
	}

	err = binary.Write(writer, binary.LittleEndian, n)
	if err != nil {
		return err
	}

	if n == 0 {
		return nil
	}

	const PyLongMarshalRatio = 15
	const PyLongMarshalBase = 32767
	const PyLongMarshalShift = 2

	size := 1 + (absInt32(n)-1)/PyLongMarshalRatio
	shortsInTopDigit := 1 + (absInt32(n)-1)%PyLongMarshalRatio

	if n < 0 {
		size = -size
	}

	var d int32
	for i := int32(0); i < size-1; i++ {
		d = 0
		for j := range PyLongMarshalRatio {
			var md int16
			err = binary.Read(reader, binary.LittleEndian, &md)
			if err != nil {
				return err
			}
			err = binary.Write(writer, binary.LittleEndian, md)
			if err != nil {
				return err
			}

			if md < 0 || md > PyLongMarshalBase {
				return errors.New("bad marshal data (digit out of range in long)")
			}
			d += int32(md) << (j * PyLongMarshalShift)
		}
	}
	d = 0
	for j := int32(0); j < shortsInTopDigit; j++ {
		var md int16
		err = binary.Read(reader, binary.LittleEndian, &md)
		if err != nil {
			return err
		}
		err = binary.Write(writer, binary.LittleEndian, md)
		if err != nil {
			return err
		}

		if md < 0 || md > PyLongMarshalBase {
			return errors.New("bad marshal data (digit out of range in long)")
		}

		if md == 0 && j == shortsInTopDigit-1 {
			return errors.New("bad marshal data (unnormalized long data)")
		}
		d += int32(md) << (j * PyLongMarshalShift)
	}

	return nil
}
