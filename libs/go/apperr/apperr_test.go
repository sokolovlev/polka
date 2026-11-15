package apperr_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc/codes"

	"github.com/sokolovlev/polka/libs/go/apperr"
)

func TestNewCarriesCodeAndReason(t *testing.T) {
	err := apperr.New(codes.FailedPrecondition, "EMAIL_NOT_VERIFIED", "verify your email first")

	assert.Equal(t, codes.FailedPrecondition, apperr.Code(err))
	assert.Equal(t, "EMAIL_NOT_VERIFIED", apperr.Reason(err))
	assert.True(t, apperr.Is(err, "EMAIL_NOT_VERIFIED"))
	assert.Equal(t, apperr.Domain, apperr.Info(err).GetDomain())
	assert.Contains(t, err.Error(), "verify your email first")
}

func TestInvalidHasField(t *testing.T) {
	err := apperr.Invalid("email", "not an email")
	assert.Equal(t, codes.InvalidArgument, apperr.Code(err))
	assert.Equal(t, apperr.ReasonValidationFailed, apperr.Reason(err))
	assert.Equal(t, "email", apperr.Info(err).GetMetadata()["field"])
}

func TestNotFound(t *testing.T) {
	err := apperr.NotFound("product")
	assert.Equal(t, codes.NotFound, apperr.Code(err))
	assert.Equal(t, apperr.ReasonNotFound, apperr.Reason(err))
}

func TestNonGRPCErrors(t *testing.T) {
	plain := errors.New("db down")
	assert.Equal(t, "", apperr.Reason(plain))
	assert.Nil(t, apperr.Info(plain))
	assert.Equal(t, codes.Unknown, apperr.Code(plain))
	assert.Equal(t, codes.OK, apperr.Code(nil))
	assert.False(t, apperr.Is(nil, "X"))

	// Wrapping keeps the status reachable (status.FromError unwraps).
	wrapped := fmt.Errorf("checkout: %w", apperr.NotFound("cart"))
	assert.Equal(t, apperr.ReasonNotFound, apperr.Reason(wrapped))
}
