package backend

import (
	"context"
	"time"

	"github.com/openbao/openbao/sdk/v2/framework"
	"github.com/openbao/openbao/sdk/v2/logical"
)

func (b *Backend) pathCreds() *framework.Path {
	return &framework.Path{
		Pattern: "creds/" + framework.GenericNameRegex("name"),
		Fields: map[string]*framework.FieldSchema{
			"name": {
				Type:        framework.TypeLowerCaseString,
				Description: "Role name",
				Required:    true,
			},
		},
		Operations: map[logical.Operation]framework.OperationHandler{
			logical.ReadOperation:   &framework.PathOperation{Callback: b.credsRead},
			logical.UpdateOperation: &framework.PathOperation{Callback: b.credsRead},
		},
		HelpSynopsis:    "Generate a Garage access key for a role.",
		HelpDescription: "Calls Garage create access key with an expiration matching the Vault lease, grants the key on the role bucket, and deletes it when the lease expires.",
	}
}

func (b *Backend) credsRead(ctx context.Context, req *logical.Request, d *framework.FieldData) (*logical.Response, error) {
	name := d.Get("name").(string)
	role, err := b.getRole(ctx, req.Storage, name)
	if err != nil {
		return nil, err
	}
	if role == nil {
		return logical.ErrorResponse("role %q does not exist", name), nil
	}
	client, cfg, err := b.getGarageClient(ctx, req.Storage)
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		return logical.ErrorResponse("garage is not configured"), nil
	}
	ttl, maxTTL := resolveTTL(role.TTL, role.MaxTTL, cfg.DefaultTTL, cfg.MaxTTL)
	expiration := time.Now().UTC().Add(ttl)
	key, err := b.provisionKey(ctx, client, role, name, expiration)
	if err != nil {
		return nil, err
	}
	resp := b.Secret(secretType).Response(map[string]interface{}{
		"access_key_id":     key.AccessKeyID,
		"secret_access_key": key.SecretAccessKey,
		"expiration":        expiration.Format(time.RFC3339),
		"bucket":            role.Bucket,
	}, map[string]interface{}{
		"access_key_id": key.AccessKeyID,
		"role":          name,
		"ttl":           int64(ttl.Seconds()),
		"max_ttl":       int64(maxTTL.Seconds()),
		"bucket":        role.Bucket,
		"read":          role.Read,
		"write":         role.Write,
		"owner":         role.Owner,
	})
	resp.Secret.TTL = ttl
	resp.Secret.MaxTTL = maxTTL
	return resp, nil
}
