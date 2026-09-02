package backend

import (
	"context"

	"github.com/ZachTech-HomeLab/vault-plugin-secrets-garage/internal/garage"
	"github.com/openbao/openbao/sdk/v2/framework"
	"github.com/openbao/openbao/sdk/v2/logical"
)

const (
	secretType            = "garage_access_key"
	backendHelp           = "The Garage secrets engine creates Garage access keys via the Garage Admin API."
	operationPrefixGarage = "garage"
)

type clientFactory func(address, token string) garage.Client

type Backend struct {
	*framework.Backend
	newClient clientFactory
}

func Factory(ctx context.Context, conf *logical.BackendConfig) (logical.Backend, error) {
	b := New()
	if err := b.Setup(ctx, conf); err != nil {
		return nil, err
	}
	return b, nil
}

func New() *Backend {
	b := &Backend{
		newClient: func(address, token string) garage.Client {
			return garage.New(address, token)
		},
	}
	b.Backend = &framework.Backend{
		Help: backendHelp,
		PathsSpecial: &logical.Paths{
			SealWrapStorage: []string{
				storageConfigKey,
				storageRolesPrefix,
			},
		},
		Paths: []*framework.Path{
			b.pathConfig(),
			b.pathRoles(),
			b.pathRolesList(),
			b.pathCreds(),
		},
		Secrets: []*framework.Secret{
			b.secretAccessKey(),
		},
		BackendType: logical.TypeLogical,
	}
	return b
}
