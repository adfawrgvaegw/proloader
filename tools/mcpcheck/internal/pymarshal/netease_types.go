package pymarshal

import (
	"bytes"
	"crypto/rc4"
	"encoding/binary"
	"errors"
)

const (
	TYPE_NETEASE_CODE_0 byte = 0x61
	TYPE_NETEASE_CODE_1 byte = 0x6f
	TYPE_NETEASE_CODE_2 byte = 0x4d

	TYPE_NETEASE_STRING_0   byte = 0x08
	TYPE_NETEASE_INTERNED_0 byte = 0x0f
	TYPE_NETEASE_UNICODE_0  byte = 0x0e

	// before 2.7
	TYPE_NETEASE_STRING_1   byte = 0x17
	TYPE_NETEASE_INTERNED_1 byte = 0x1a
	TYPE_NETEASE_UNICODE_1  byte = 0x1d

	// after 2.7
	TYPE_NETEASE_STRING_2   byte = 0x6d
	TYPE_NETEASE_INTERNED_2 byte = 0x62
	TYPE_NETEASE_UNICODE_2  byte = 0x31
)

func repairNetEaseString1(reader *bytes.Reader, writer *bytes.Buffer, write bool) error {
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

	decrypt := make([]byte, n)

	rc4Cipher, err := rc4.NewCipher([]byte{0xA7, 0xD, 0x37, 0x7A})
	if err != nil {
		return err
	}

	rc4Cipher.XORKeyStream(decrypt, data)

	_, err = writer.Write(decrypt)
	if err != nil {
		return err
	}

	if !write {
		strings = append(strings, decrypt)
	}

	return nil
}

func repairNetEaseString2(reader *bytes.Reader, writer *bytes.Buffer, write bool) error {
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

	decrypt := make([]byte, n)

	rc4Cipher, err := rc4.NewCipher([]byte{0x8d, 0x06, 0xe8, 0xc8, 0xb7, 0xd7, 0xb7, 0x28, 0x46, 0x51, 0xae, 0x04})
	if err != nil {
		return err
	}

	rc4Cipher.XORKeyStream(decrypt, data)

	_, err = writer.Write(decrypt)
	if err != nil {
		return err
	}

	if !write {
		strings = append(strings, decrypt)
	}

	return nil
}

func repairNetEaseString0(reader *bytes.Reader, writer *bytes.Buffer, write bool) error {
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

	for i := 0; i < len(data); i++ {
		data[i] ^= 0x8d
	}

	_, err = writer.Write(data)
	if err != nil {
		return err
	}

	if !write {
		strings = append(strings, data)
	}

	return nil
}

func repairOpcode(reader *bytes.Reader, writer *bytes.Buffer, id uint32) error {
	err := copyBytes(reader, writer, 1)
	if err != nil {
		return err
	}

	var n int32
	err = binary.Read(reader, binary.LittleEndian, &n)
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

	if opcodeMap, ok := neteaseOpcodes[id]; ok {
		index := 0
		for index < len(data) {
			q := opcodeMap[data[index]]
			if q != "" {
				p, ok := opcodes[q]
				if ok {
					data[index] = p
					if p >= 90 {
						index += 2
					}
				} else {
					//println("unknown:", data[index], q)
				}
			} else {
				//println("unknown:", data[index])
			}
			index++
		}

		_, err = writer.Write(data)
		return err
	} else {
		return errors.New("no opcode map found")
	}
}

func writeCodeObject(argcount, locals, stacksize, flags, ncode, consts, names, varnames, freevars, cellvars, filename, name, firstlineno, lnotab []byte, writer *bytes.Buffer) error {
	_, err := writer.Write(argcount)
	if err != nil {
		return err
	}
	_, err = writer.Write(locals)
	if err != nil {
		return err
	}
	_, err = writer.Write(stacksize)
	if err != nil {
		return err
	}
	_, err = writer.Write(flags)
	if err != nil {
		return err
	}
	_, err = writer.Write(ncode)
	if err != nil {
		return err
	}
	_, err = writer.Write(consts)
	if err != nil {
		return err
	}
	_, err = writer.Write(names)
	if err != nil {
		return err
	}
	_, err = writer.Write(varnames)
	if err != nil {
		return err
	}
	_, err = writer.Write(freevars)
	if err != nil {
		return err
	}
	_, err = writer.Write(cellvars)
	if err != nil {
		return err
	}
	_, err = writer.Write(filename)
	if err != nil {
		return err
	}
	_, err = writer.Write(name)
	if err != nil {
		return err
	}
	_, err = writer.Write(firstlineno)
	if err != nil {
		return err
	}
	_, err = writer.Write(lnotab)
	if err != nil {
		return err
	}

	return nil
}
