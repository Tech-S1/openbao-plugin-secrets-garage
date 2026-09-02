package backend

import (
	"context"
	"time"

	"github.com/openbao/openbao/sdk/v2/framework"
	"github.com/openbao/openbao/sdk/v2/logical"
)

func (b *Backend) pathConfig() *framework.Path {
	return &framework.Path{
		Pattern: "config",
		DisplayAttrs: &framework.DisplayAttributes{
			OperationPrefix: operationPrefixGarage,
		},
		Fields: map[string]*framework.FieldSchema{
			"address": {
				Type:        framework.TypeString,
				Description: "Garage Admin API base URL",
				Required:    true,
				DisplayAttrs: &framework.DisplayAttributes{
					Name: "Garage Admin API URL",
				},
			},
			"token": {
				Type:        framework.TypeString,
				Description: "Garage Admin API bearer token",
				DisplayAttrs: &framework.DisplayAttributes{
					Name:      "Garage Admin Token",
					Sensitive: true,
				},
			},
			"default_ttl": {
				Type:        framework.TypeDurationSecond,
				Description: "Default lease TTL for generated keys",
				Default:     300,
				DisplayAttrs: &framework.DisplayAttributes{
					Name: "Default TTL",
				},
			},
			"max_ttl": {
				Type:        framework.TypeDurationSecond,
				Description: "Max lease TTL for generated keys",
				Default:     900,
				DisplayAttrs: &framework.DisplayAttributes{
					Name: "Max TTL",
				},
			},
		},
		Operations: map[logical.Operation]framework.OperationHandler{
			logical.ReadOperation: &framework.PathOperation{
				Callback: b.configRead,
				DisplayAttrs: &framework.DisplayAttributes{
					OperationSuffix: "configuration",
				},
			},
			logical.UpdateOperation: &framework.PathOperation{
				Callback: b.configWrite,
				DisplayAttrs: &framework.DisplayAttributes{
					OperationVerb: "configure",
				},
			},
			logical.CreateOperation: &framework.PathOperation{
				Callback: b.configWrite,
				DisplayAttrs: &framework.DisplayAttributes{
					OperationVerb: "configure",
				},
			},
			logical.DeleteOperation: &framework.PathOperation{
				Callback: b.configDelete,
				DisplayAttrs: &framework.DisplayAttributes{
					OperationSuffix: "configuration",
				},
			},
		},
		ExistenceCheck:  b.configExistence,
		HelpSynopsis:    "Configure the Garage Admin API connection.",
		HelpDescription: "Stores the Garage Admin API address and token used to create and delete access keys.",
	}
}

func (b *Backend) configExistence(ctx context.Context, req *logical.Request, _ *framework.FieldData) (bool, error) {
	cfg, err := b.getConfig(ctx, req.Storage)
	if err != nil {
		return false, err
	}
	return cfg != nil, nil
}

func (b *Backend) configRead(ctx context.Context, req *logical.Request, _ *framework.FieldData) (*logical.Response, error) {
	cfg, err := b.getConfig(ctx, req.Storage)
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		return nil, nil
	}
	return &logical.Response{
		Data: map[string]interface{}{
			"address":     cfg.Address,
			"default_ttl": int64(cfg.DefaultTTL.Seconds()),
			"max_ttl":     int64(cfg.MaxTTL.Seconds()),
		},
	}, nil
}

func (b *Backend) configWrite(ctx context.Context, req *logical.Request, d *framework.FieldData) (*logical.Response, error) {
	cfg, err := b.getConfig(ctx, req.Storage)
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		cfg = &Config{
			DefaultTTL: defaultTTL,
			MaxTTL:     defaultMaxTTL,
		}
	}
	if v, ok := d.GetOk("address"); ok {
		cfg.Address = v.(string)
	}
	if v, ok := d.GetOk("token"); ok {
		cfg.Token = v.(string)
	}
	if v, ok := d.GetOk("default_ttl"); ok {
		cfg.DefaultTTL = time.Duration(v.(int)) * time.Second
	}
	if v, ok := d.GetOk("max_ttl"); ok {
		cfg.MaxTTL = time.Duration(v.(int)) * time.Second
	}
	if cfg.Address == "" {
		return logical.ErrorResponse("address is required"), nil
	}
	if cfg.Token == "" {
		return logical.ErrorResponse("token is required"), nil
	}
	if cfg.MaxTTL > 0 && cfg.DefaultTTL > cfg.MaxTTL {
		return logical.ErrorResponse("default_ttl cannot be greater than max_ttl"), nil
	}
	client := b.newClient(cfg.Address, cfg.Token)
	if resp := configValidationError(client.ValidateConnection(ctx)); resp != nil {
		return resp, nil
	}
	entry, err := logical.StorageEntryJSON(storageConfigKey, cfg)
	if err != nil {
		return nil, err
	}
	if err := req.Storage.Put(ctx, entry); err != nil {
		return nil, err
	}
	return nil, nil
}

func (b *Backend) configDelete(ctx context.Context, req *logical.Request, _ *framework.FieldData) (*logical.Response, error) {
	return nil, req.Storage.Delete(ctx, storageConfigKey)
}
