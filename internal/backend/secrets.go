package backend

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/openbao/openbao/sdk/v2/framework"
	"github.com/openbao/openbao/sdk/v2/logical"
)

func (b *Backend) secretAccessKey() *framework.Secret {
	return &framework.Secret{
		Type: secretType,
		Fields: map[string]*framework.FieldSchema{
			"access_key_id": {
				Type:        framework.TypeString,
				Description: "Garage S3 access key id",
			},
			"secret_access_key": {
				Type:        framework.TypeString,
				Description: "Garage S3 secret access key",
			},
			"expiration": {
				Type:        framework.TypeString,
				Description: "Garage key expiration (RFC3339)",
			},
			"bucket": {
				Type:        framework.TypeString,
				Description: "Bucket the key was granted on",
			},
		},
		Revoke: b.secretRevoke,
		Renew:  b.secretRenew,
	}
}

func (b *Backend) secretRevoke(ctx context.Context, req *logical.Request, _ *framework.FieldData) (*logical.Response, error) {
	client, cfg, err := b.getGarageClient(ctx, req.Storage)
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		return nil, fmt.Errorf("garage is not configured")
	}
	id, err := secretString(req.Secret.InternalData, "access_key_id")
	if err != nil {
		return nil, err
	}
	if err := client.DeleteKeyWithRetry(ctx, id); err != nil {
		return nil, fmt.Errorf("DeleteKey: %w", err)
	}
	return nil, nil
}

func (b *Backend) secretRenew(ctx context.Context, req *logical.Request, _ *framework.FieldData) (*logical.Response, error) {
	client, cfg, err := b.getGarageClient(ctx, req.Storage)
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		return nil, fmt.Errorf("garage is not configured")
	}
	ttl, err := snapshotDuration(req.Secret.InternalData, "ttl")
	if err != nil {
		return nil, err
	}
	maxTTL, err := snapshotDuration(req.Secret.InternalData, "max_ttl")
	if err != nil {
		return nil, err
	}
	id, err := secretString(req.Secret.InternalData, "access_key_id")
	if err != nil {
		return nil, err
	}
	if err := client.UpdateKey(ctx, id, time.Now().UTC().Add(ttl)); err != nil {
		return nil, fmt.Errorf("UpdateKey: %w", err)
	}
	resp := &logical.Response{Secret: req.Secret}
	resp.Secret.TTL = ttl
	resp.Secret.MaxTTL = maxTTL
	return resp, nil
}

func secretString(data map[string]interface{}, key string) (string, error) {
	raw, ok := data[key]
	if !ok {
		return "", fmt.Errorf("secret is missing %s", key)
	}
	s, ok := raw.(string)
	if !ok || s == "" {
		return "", fmt.Errorf("secret has invalid %s", key)
	}
	return s, nil
}

func snapshotDuration(data map[string]interface{}, key string) (time.Duration, error) {
	raw, ok := data[key]
	if !ok {
		return 0, fmt.Errorf("secret is missing %s", key)
	}
	switch v := raw.(type) {
	case int64:
		return time.Duration(v) * time.Second, nil
	case int:
		return time.Duration(v) * time.Second, nil
	case float64:
		return time.Duration(int64(v)) * time.Second, nil
	case json.Number:
		n, err := v.Int64()
		if err != nil {
			return 0, fmt.Errorf("secret has invalid %s", key)
		}
		return time.Duration(n) * time.Second, nil
	default:
		return 0, fmt.Errorf("secret has invalid %s", key)
	}
}
