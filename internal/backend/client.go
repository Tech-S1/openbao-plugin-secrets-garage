package backend

import (
	"context"

	"github.com/ZachTech-HomeLab/vault-plugin-secrets-garage/internal/garage"
	"github.com/openbao/openbao/sdk/v2/logical"
)

func (b *Backend) getGarageClient(ctx context.Context, s logical.Storage) (garage.Client, *Config, error) {
	cfg, err := b.getConfig(ctx, s)
	if err != nil {
		return nil, nil, err
	}
	if cfg == nil {
		return nil, nil, nil
	}
	return b.newClient(cfg.Address, cfg.Token), cfg, nil
}
