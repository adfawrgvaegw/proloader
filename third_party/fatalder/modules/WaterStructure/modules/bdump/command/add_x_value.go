package command

import (
	"io"
)

type AddXValue struct{}

func (_ *AddXValue) ID() uint16 {
	return IDAddXValue
}

func (_ *AddXValue) Name() string {
	return NameAddXValue
}

func (_ *AddXValue) Marshal(_ io.Writer) error {
	return nil
}

func (_ *AddXValue) Unmarshal(_ io.Reader) error {
	return nil
}
