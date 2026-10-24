package command

import (
	"io"
)

type AddZValue0 struct{}

func (_ *AddZValue0) ID() uint16 {
	return IDAddZValue0
}

func (_ *AddZValue0) Name() string {
	return NameAddZValue0
}

func (_ *AddZValue0) Marshal(_ io.Writer) error {
	return nil
}

func (_ *AddZValue0) Unmarshal(_ io.Reader) error {
	return nil
}
