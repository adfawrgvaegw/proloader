package pymarshal

import (
	"bytes"
	"encoding/binary"
)

func repairNetEaseCode1(reader *bytes.Reader, writer *bytes.Buffer) error {
	locals := make([]byte, 4)
	_, err := reader.Read(locals)
	if err != nil {
		return err
	}

	flags := make([]byte, 4)
	_, err = reader.Read(flags)
	if err != nil {
		return err
	}

	var consts bytes.Buffer
	err = RepairMarshal(reader, &consts)
	if err != nil {
		return err
	}

	stacksize := make([]byte, 4)
	_, err = reader.Read(stacksize)
	if err != nil {
		return err
	}

	var varnames bytes.Buffer
	err = RepairMarshal(reader, &varnames)
	if err != nil {
		return err
	}

	argcount := make([]byte, 4)
	_, err = reader.Read(argcount)
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

	var cellvars bytes.Buffer
	err = RepairMarshal(reader, &cellvars)
	if err != nil {
		return err
	}

	var name bytes.Buffer
	err = RepairMarshal(reader, &name)
	if err != nil {
		return err
	}

	var code bytes.Buffer
	err = RepairMarshal(reader, &code)
	if err != nil {
		return err
	}

	firstlineno := make([]byte, 4)
	_, err = reader.Read(firstlineno)
	if err != nil {
		return err
	}

	var lnotab bytes.Buffer
	err = RepairMarshal(reader, &lnotab)
	if err != nil {
		return err
	}

	var opcodeType uint32
	err = binary.Read(reader, binary.LittleEndian, &opcodeType)
	if err != nil {
		return err
	}
	//fmt.Printf("opcode: %x %d\n", opcodeType, opcodeType)
	var ncode bytes.Buffer
	err = repairOpcode(bytes.NewReader(code.Bytes()), &ncode, opcodeType)
	if err != nil {
		return err
	}

	var filename bytes.Buffer
	err = RepairMarshal(reader, &filename)
	if err != nil {
		return err
	}

	return writeCodeObject(argcount, locals, stacksize, flags, ncode.Bytes(), consts.Bytes(), names.Bytes(), varnames.Bytes(), freevars.Bytes(), cellvars.Bytes(), filename.Bytes(), name.Bytes(), firstlineno, lnotab.Bytes(), writer)
}
