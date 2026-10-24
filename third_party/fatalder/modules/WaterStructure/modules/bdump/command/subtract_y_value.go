package command

import (
	"io"
)

type SubtractYValue struct{}

func (_ *SubtractYValue) ID() uint16 {
	return IDSubtractYValue
}

func (_ *SubtractYValue) Name() string {
	return NameSubtractYValue
}

func (_ *SubtractYValue) Marshal(_ io.Writer) error {
	return nil
}

func (_ *SubtractYValue) Unmarshal(_ io.Reader) error {
	return nil
}
