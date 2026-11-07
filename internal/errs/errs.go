package errs

import "errors"

var (
	ErrConflict        = errors.New("conflict")
	ErrNotFound        = errors.New("not found")
	ErrForbidden       = errors.New("forbidden")
	ErrUnauthenticated = errors.New("unauthenticated")
	ErrInvalid         = errors.New("invalid")
	ErrDisabled        = errors.New("disabled")
	ErrReuse           = errors.New("refresh reuse")
)
