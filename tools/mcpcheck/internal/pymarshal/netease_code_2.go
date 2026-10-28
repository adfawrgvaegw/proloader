/**
 * This file is part of the UnMCPK project.
 *
 * Unauthorized copying of this file, via any medium is strictly prohibited.
 *
 * @auther nekomimigirl
**/
package pymarshal

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

func repairNetEaseCode2(reader *bytes.Reader, writer *bytes.Buffer) error {
	argcount := make([]byte, 4)
	_, err := reader.Read(argcount)
	if err != nil {
		return err
	}

	var lnotab bytes.Buffer
	err = RepairMarshal(reader, &lnotab)
	if err != nil {
		return err
	}

	var cellvars bytes.Buffer
	err = RepairMarshal(reader, &cellvars)
	if err != nil {
		return err
	}

	firstlineno := make([]byte, 4)
	_, err = reader.Read(firstlineno)
	if err != nil {
		return err
	}

	var varnames bytes.Buffer
	err = RepairMarshal(reader, &varnames)
	if err != nil {
		return err
	}

	var consts bytes.Buffer
	removedCode := false
	if depth == 1 {
		// clean up constant pool
		if type1, _ := reader.ReadByte(); type1 == TYPE_TUPLE {
			var n int32
			err := binary.Read(reader, binary.LittleEndian, &n)
			if err != nil {
				return err
			}

			data := make([]bytes.Buffer, n)
			codeIndex := int32(-1)

			for i := int32(0); i < n; i++ {
				err = RepairMarshal(reader, &data[i])
				if err != nil {
					return err
				}

				code := data[i].Bytes()
				if code[0] == TYPE_CODE /* && bytes.Contains(code, []byte("newVersi0n"))*/ {
					codeIndex = i
				}
			}

			consts.WriteByte(TYPE_TUPLE)
			err = binary.Write(&consts, binary.LittleEndian, n)
			if err != nil {
				return err
			}

			for i := 0; i < len(data); i++ {
				if i == int(codeIndex) {
					consts.Write([]byte{TYPE_NONE})
					removedCode = true
				} else {
					consts.Write(data[i].Bytes())
				}
			}
		} else {
			return fmt.Errorf("invalid constant pool type: %x", type1)
		}
	} else {
		err = RepairMarshal(reader, &consts)
		if err != nil {
			return err
		}
	}

	var name bytes.Buffer
	err = RepairMarshal(reader, &name)
	if err != nil {
		return err
	}

	stacksize := make([]byte, 4)
	_, err = reader.Read(stacksize)
	if err != nil {
		return err
	}

	var freevars bytes.Buffer
	err = RepairMarshal(reader, &freevars)
	if err != nil {
		return err
	}

	var names bytes.Buffer
	err = RepairMarshal(reader, &names)
	if err != nil {
		return err
	}

	var code bytes.Buffer
	err = RepairMarshal(reader, &code)
	if err != nil {
		return err
	}

	flags := make([]byte, 4)
	_, err = reader.Read(flags)
	if err != nil {
		return err
	}

	var filename bytes.Buffer
	err = RepairMarshal(reader, &filename)
	if err != nil {
		return err
	}

	locals := make([]byte, 4)
	_, err = reader.Read(locals)
	if err != nil {
		return err
	}

	var opcodeType uint32
	err = binary.Read(reader, binary.LittleEndian, &opcodeType)
	if err != nil {
		return err
	}
	//fmt.Printf("opcode: %x %d\n", opcodeType, opcodeType)
	if opcodeMap, ok := neteaseOpcodes[opcodeType]; ok {
		data := code.Bytes()
		index := 5
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
		if depth == 1 && removedCode {
			data = append(data[:len(data)-3*3-1], data[len(data)-1])
			var buf bytes.Buffer
			var n int32
			err := binary.Read(bytes.NewReader(data[1:5]), binary.LittleEndian, &n)
			if err != nil {
				return err
			}

			err = binary.Write(&buf, binary.LittleEndian, n-3*3)
			if err != nil {
				return err
			}

			num := buf.Bytes()
			data[1] = num[0]
			data[2] = num[1]
			data[3] = num[2]
			data[4] = num[3]
		}
		code = *bytes.NewBuffer(data)
	} else {
		//fmt.Printf("No opcode mapping found %x\n", opcodeType)
	}

	return writeCodeObject(argcount, locals, stacksize, flags, code.Bytes(), consts.Bytes(), names.Bytes(), varnames.Bytes(), freevars.Bytes(), cellvars.Bytes(), filename.Bytes(), name.Bytes(), firstlineno, lnotab.Bytes(), writer)
}
