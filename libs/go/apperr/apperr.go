// Package apperr builds and reads gRPC errors with a machine-readable reason
// (spec 6.1): a standard status code plus google.rpc.ErrorInfo, e.g.
// FAILED_PRECONDITION + reason EMAIL_NOT_VERIFIED. The gateway maps them to
// HTTP status and the Problem Details `code`.
package apperr

import (
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Domain identifies Polka in ErrorInfo.
const Domain = "polka"

// New returns a gRPC error with code, reason and a human-readable message.
// metadata is optional context such as {"field": "email"}.
func New(code codes.Code, reason, msg string, metadata ...map[string]string) error {
	st := status.New(code, msg)
	info := &errdetails.ErrorInfo{Reason: reason, Domain: Domain}
	if len(metadata) > 0 {
		info.Metadata = metadata[0]
	}
	withInfo, err := st.WithDetails(info)
	if err != nil {
		// Only fails for an OK status; fall back to the bare status.
		return st.Err() //nolint:wrapcheck // gRPC status errors must reach the transport unwrapped
	}
	return withInfo.Err() //nolint:wrapcheck // gRPC status errors must reach the transport unwrapped
}

// Reason returns the ErrorInfo reason of err ("" if none).
func Reason(err error) string {
	if info := Info(err); info != nil {
		return info.GetReason()
	}
	return ""
}

// Info returns the ErrorInfo of err, or nil.
func Info(err error) *errdetails.ErrorInfo {
	st, ok := status.FromError(err)
	if !ok {
		return nil
	}
	for _, d := range st.Details() {
		if info, ok := d.(*errdetails.ErrorInfo); ok {
			return info
		}
	}
	return nil
}

// Code returns the gRPC code of err: codes.OK for nil, codes.Unknown for a
// non-gRPC error.
func Code(err error) codes.Code {
	return status.Code(err)
}

// Is reports whether err is a gRPC error with the given reason.
func Is(err error, reason string) bool {
	return err != nil && Reason(err) == reason
}

// Reasons shared by several services; service-specific reasons (OUT_OF_STOCK,
// SHOP_EXISTS, ...) are defined next to the code that returns them.
const (
	ReasonNotFound         = "NOT_FOUND"
	ReasonValidationFailed = "VALIDATION_FAILED"
	ReasonForbidden        = "FORBIDDEN"
	ReasonUnauthenticated  = "UNAUTHENTICATED"
)

// NotFound is a NOT_FOUND error with reason NOT_FOUND.
func NotFound(msg string) error { return New(codes.NotFound, ReasonNotFound, msg) }

// Invalid is an INVALID_ARGUMENT error with reason VALIDATION_FAILED and
// the offending field in metadata.
func Invalid(field, msg string) error {
	return New(codes.InvalidArgument, ReasonValidationFailed, msg, map[string]string{"field": field})
}
