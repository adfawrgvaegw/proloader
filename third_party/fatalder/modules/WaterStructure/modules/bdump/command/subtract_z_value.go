package command

import (
	"io"
)

type SubtractZValue struct{}

func (_ *SubtractZValue) ID() uint16 {
	return IDSubtractZValue
}

func (_ *SubtractZValue) Name() string {
	return NameSubtractZValue
}

func (_ *SubtractZValue) Marshal(_ io.Writer) error {
	return nil
}

func (_ *SubtractZValue) Unmarshal(_ io.Reader) error {
	return nil
}
