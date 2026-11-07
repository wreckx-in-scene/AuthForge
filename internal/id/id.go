package id

import (
	"github.com/google/uuid"

	"github.com/wreckx-in-scene/AuthForge/internal/errs"
)

func Parse(values ...string) error {
	for _, value := range values {
		if _, err := uuid.Parse(value); err != nil {
			return errs.ErrInvalid
		}
	}
	return nil
}
