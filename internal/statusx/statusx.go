package statusx

import (
	"errors"
	"log/slog"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/wreckx-in-scene/AuthForge/internal/errs"
)

func Fail(log *slog.Logger, err error, conflict string) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, errs.ErrConflict):
		if conflict == "" {
			conflict = "already_exists"
		}
		return status.Error(codes.AlreadyExists, conflict)
	case errors.Is(err, errs.ErrNotFound):
		return status.Error(codes.NotFound, "not_found")
	case errors.Is(err, errs.ErrForbidden):
		return status.Error(codes.PermissionDenied, "forbidden")
	case errors.Is(err, errs.ErrUnauthenticated):
		return status.Error(codes.Unauthenticated, "invalid_credentials")
	case errors.Is(err, errs.ErrReuse):
		return status.Error(codes.FailedPrecondition, "refresh_reuse_detected")
	case errors.Is(err, errs.ErrDisabled):
		return status.Error(codes.PermissionDenied, "account_disabled")
	case errors.Is(err, errs.ErrInvalid):
		return status.Error(codes.InvalidArgument, "invalid_argument")
	default:
		log.Error("request failed", "err", err)
		return status.Error(codes.Internal, "internal")
	}
}
