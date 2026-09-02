package backend

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"

	"github.com/ZachTech-HomeLab/vault-plugin-secrets-garage/internal/garage"
	"github.com/openbao/openbao/sdk/v2/logical"
)

func configValidationError(err error) *logical.Response {
	if err == nil {
		return nil
	}
	if apiErr, ok := garage.AsAPIError(err); ok {
		switch apiErr.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			return logical.ErrorResponse("invalid Garage admin token")
		case http.StatusNotFound:
			return logical.ErrorResponse("Garage admin API not found at the configured address")
		default:
			if apiErr.StatusCode >= 400 && apiErr.StatusCode < 500 {
				return logical.ErrorResponse("Garage rejected the connection: %s", apiErr.Message)
			}
		}
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return logical.ErrorResponse("cannot reach Garage at the configured address: %v", err)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return logical.ErrorResponse("timed out connecting to Garage at the configured address")
	}
	return logical.ErrorResponse("failed to validate Garage connection: %v", err)
}

func bucketValidationError(err error, bucket string) (*logical.Response, error) {
	if err == nil {
		return nil, nil
	}
	if apiErr, ok := garage.AsAPIError(err); ok {
		if apiErr.StatusCode == http.StatusNotFound {
			return logical.ErrorResponse("bucket %q does not exist in Garage", bucket), nil
		}
		if apiErr.StatusCode >= 400 && apiErr.StatusCode < 500 {
			return logical.ErrorResponse("failed to validate bucket %q: %s", bucket, apiErr.Message), nil
		}
	}
	return nil, fmt.Errorf("GetBucketInfo: %w", err)
}
