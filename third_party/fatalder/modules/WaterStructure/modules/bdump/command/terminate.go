package command

import "io"

type Terminate struct{}

func (_ *Terminate) ID() uint16 {
	return IDTerminate
}

func (_ *Terminate) Name() string {
	return NameTerminate
}

func (_ *Terminate) Marshal(_ io.Writer) error {
	return nil
}

func (_ *Terminate) Unmarshal(_ io.Reader) error {
	return nil
}
