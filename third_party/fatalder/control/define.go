package control

import (
	"errors"
)

var ErrAlreadyConnected = errors.New("already connected")
var ErrNotConnected = errors.New("not connected")
