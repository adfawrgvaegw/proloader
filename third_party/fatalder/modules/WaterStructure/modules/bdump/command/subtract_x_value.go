package command

import (
	"io"
)

type SubtractXValue struct{}

func (_ *SubtractXValue) ID() uint16 {
	return IDSubtractXValue
}

func (_ *SubtractXValue) Name() string {
	return NameSubtractXValue
}

func (_ *SubtractXValue) Marshal(_ io.Writer) error {
	return nil
}

func (_ *SubtractXValue) Unmarshal(_ io.Reader) error {
	return nil
}
