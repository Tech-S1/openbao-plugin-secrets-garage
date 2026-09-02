package garage_test

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"testing"

	"github.com/ZachTech-HomeLab/vault-plugin-secrets-garage/internal/garage"
)

func TestIsRetryable(t *testing.T) {
	t.Parallel()

	apiErr := func(code int) error {
		return &garage.APIError{StatusCode: code}
	}

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{name: "network", err: &net.DNSError{IsTimeout: true}, want: true},
		{name: "500", err: apiErr(http.StatusInternalServerError), want: true},
		{name: "429", err: apiErr(http.StatusTooManyRequests), want: true},
		{name: "404", err: apiErr(http.StatusNotFound), want: false},
		{name: "401", err: apiErr(http.StatusUnauthorized), want: false},
		{name: "generic", err: errors.New("boom"), want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := garage.IsRetryable(tc.err); got != tc.want {
				t.Fatalf("IsRetryable() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestAsAPIError(t *testing.T) {
	t.Parallel()

	inner := &garage.APIError{
		Method:     http.MethodGet,
		Path:       "/v2/ListKeys",
		StatusCode: http.StatusForbidden,
		Message:    "forbidden",
	}

	apiErr, ok := garage.AsAPIError(fmt.Errorf("wrapped: %w", inner))
	if !ok {
		t.Fatal("expected API error")
	}
	if apiErr.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", apiErr.StatusCode, http.StatusForbidden)
	}

	if _, ok := garage.AsAPIError(errors.New("other")); ok {
		t.Fatal("expected false for non-API error")
	}
}

func TestAPIErrorString(t *testing.T) {
	t.Parallel()

	err := &garage.APIError{
		Method:     http.MethodPost,
		Path:       "/v2/DeleteKey",
		StatusCode: http.StatusNotFound,
		Message:    "not found",
	}
	want := "garage POST /v2/DeleteKey: status 404: not found"
	if err.Error() != want {
		t.Fatalf("Error() = %q, want %q", err.Error(), want)
	}
}
