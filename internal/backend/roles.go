package backend

import (
	"context"
	"time"

	"github.com/openbao/openbao/sdk/v2/framework"
	"github.com/openbao/openbao/sdk/v2/logical"
)

func (b *Backend) pathRolesList() *framework.Path {
	return &framework.Path{
		Pattern: "roles/?$",
		Operations: map[logical.Operation]framework.OperationHandler{
			logical.ListOperation: &framework.PathOperation{Callback: b.rolesList},
		},
		HelpSynopsis:    "List Garage credential roles.",
		HelpDescription: "Lists roles that map a Vault lease to a Garage bucket and key permissions.",
	}
}

func (b *Backend) pathRoles() *framework.Path {
	return &framework.Path{
		Pattern: "roles/" + framework.GenericNameRegex("name"),
		Fields: map[string]*framework.FieldSchema{
			"name": {
				Type:        framework.TypeLowerCaseString,
				Description: "Role name",
				Required:    true,
			},
			"bucket": {
				Type:        framework.TypeString,
				Description: "Garage bucket global alias to grant the key on",
				Required:    true,
			},
			"ttl": {
				Type:        framework.TypeDurationSecond,
				Description: "Lease TTL for keys from this role",
			},
			"max_ttl": {
				Type:        framework.TypeDurationSecond,
				Description: "Max lease TTL for keys from this role",
			},
			"read": {
				Type:        framework.TypeBool,
				Description: "Grant read on the bucket",
				Default:     true,
			},
			"write": {
				Type:        framework.TypeBool,
				Description: "Grant write on the bucket",
				Default:     true,
			},
			"owner": {
				Type:        framework.TypeBool,
				Description: "Grant owner on the bucket",
				Default:     false,
			},
		},
		Operations: map[logical.Operation]framework.OperationHandler{
			logical.ReadOperation:   &framework.PathOperation{Callback: b.roleRead},
			logical.UpdateOperation: &framework.PathOperation{Callback: b.roleWrite},
			logical.CreateOperation: &framework.PathOperation{Callback: b.roleWrite},
			logical.DeleteOperation: &framework.PathOperation{Callback: b.roleDelete},
		},
		ExistenceCheck:  b.roleExistence,
		HelpSynopsis:    "Manage Garage credential roles.",
		HelpDescription: "A role sets the bucket, permissions, and TTL used when reading creds.",
	}
}

func (b *Backend) roleExistence(ctx context.Context, req *logical.Request, d *framework.FieldData) (bool, error) {
	role, err := b.getRole(ctx, req.Storage, d.Get("name").(string))
	if err != nil {
		return false, err
	}
	return role != nil, nil
}

func (b *Backend) rolesList(ctx context.Context, req *logical.Request, _ *framework.FieldData) (*logical.Response, error) {
	entries, err := req.Storage.List(ctx, storageRolesPrefix)
	if err != nil {
		return nil, err
	}
	return logical.ListResponse(entries), nil
}

func (b *Backend) roleRead(ctx context.Context, req *logical.Request, d *framework.FieldData) (*logical.Response, error) {
	role, err := b.getRole(ctx, req.Storage, d.Get("name").(string))
	if err != nil {
		return nil, err
	}
	if role == nil {
		return nil, nil
	}
	return &logical.Response{
		Data: map[string]interface{}{
			"bucket":  role.Bucket,
			"ttl":     int64(role.TTL.Seconds()),
			"max_ttl": int64(role.MaxTTL.Seconds()),
			"read":    role.Read,
			"write":   role.Write,
			"owner":   role.Owner,
		},
	}, nil
}

func (b *Backend) roleWrite(ctx context.Context, req *logical.Request, d *framework.FieldData) (*logical.Response, error) {
	name := d.Get("name").(string)
	role, err := b.getRole(ctx, req.Storage, name)
	if err != nil {
		return nil, err
	}
	if role == nil {
		role = &Role{
			Read:  true,
			Write: true,
		}
	}
	if v, ok := d.GetOk("bucket"); ok {
		role.Bucket = v.(string)
	}
	if v, ok := d.GetOk("ttl"); ok {
		role.TTL = time.Duration(v.(int)) * time.Second
	}
	if v, ok := d.GetOk("max_ttl"); ok {
		role.MaxTTL = time.Duration(v.(int)) * time.Second
	}
	if v, ok := d.GetOk("read"); ok {
		role.Read = v.(bool)
	}
	if v, ok := d.GetOk("write"); ok {
		role.Write = v.(bool)
	}
	if v, ok := d.GetOk("owner"); ok {
		role.Owner = v.(bool)
	}
	if role.Bucket == "" {
		return logical.ErrorResponse("bucket is required"), nil
	}
	if role.MaxTTL > 0 && role.TTL > role.MaxTTL {
		return logical.ErrorResponse("ttl cannot be greater than max_ttl"), nil
	}
	client, cfg, err := b.getGarageClient(ctx, req.Storage)
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		return logical.ErrorResponse("garage is not configured"), nil
	}
	if _, err := client.GetBucketInfo(ctx, role.Bucket); err != nil {
		if resp, respErr := bucketValidationError(err, role.Bucket); resp != nil || respErr != nil {
			return resp, respErr
		}
	}
	entry, err := logical.StorageEntryJSON(storageRolesPrefix+name, role)
	if err != nil {
		return nil, err
	}
	if err := req.Storage.Put(ctx, entry); err != nil {
		return nil, err
	}
	return nil, nil
}

func (b *Backend) roleDelete(ctx context.Context, req *logical.Request, d *framework.FieldData) (*logical.Response, error) {
	return nil, req.Storage.Delete(ctx, storageRolesPrefix+d.Get("name").(string))
}
