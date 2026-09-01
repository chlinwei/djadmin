package apperror

import (
	"errors"
	"net/http"
	"testing"
)

func TestWithCausePreservesCatalogError(t *testing.T) {
	cause := errors.New("database unavailable")
	base := NewWithHTTP(6101, "查询用户失败", http.StatusInternalServerError)
	wrapped := WithCause(base, cause)

	if wrapped == base {
		t.Fatal("WithCause must not mutate or return the global catalog error")
	}
	if wrapped.Code() != base.Code() || wrapped.Message() != base.Message() || wrapped.HTTPStatus() != base.HTTPStatus() {
		t.Fatalf("wrapped error changed public contract: %+v", wrapped)
	}
	if !errors.Is(wrapped, cause) {
		t.Fatal("wrapped error must retain the internal cause")
	}
}

func TestAsRejectsOrdinaryErrors(t *testing.T) {
	if _, ok := As(errors.New("plain error")); ok {
		t.Fatal("ordinary errors must not be exposed as application errors")
	}
}
