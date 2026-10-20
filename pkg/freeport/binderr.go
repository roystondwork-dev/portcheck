package freeport

import (
	"errors"
	"io/fs"
)

type bindResult int

const (
	bindOK bindResult = iota
	bindInUse
	bindDenied
	bindSkip // address unusable on this system; ignore it
)

// classifyBindErr maps a bind error to a verdict. Unknown errors are treated
// as "in use" so that a port is never reported free when binding failed.
func classifyBindErr(err error) bindResult {
	switch {
	case err == nil:
		return bindOK
	case errors.Is(err, errAddrInUse):
		return bindInUse
	case errors.Is(err, errAddrNotAvail), errors.Is(err, errAFNoSupport):
		return bindSkip
	case errors.Is(err, fs.ErrPermission), errors.Is(err, errAccess):
		return bindDenied
	default:
		return bindInUse
	}
}
