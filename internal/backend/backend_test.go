package backend_test

import (
	"context"
	"testing"

	"github.com/ZachTech-HomeLab/vault-plugin-secrets-garage/internal/backend"
	"github.com/ZachTech-HomeLab/vault-plugin-secrets-garage/internal/garage/garagetest"
	"github.com/openbao/openbao/sdk/v2/logical"
	"github.com/stretchr/testify/require"
)

type fixture struct {
	Backend *backend.Backend
	Storage *logical.InmemStorage
	Garage  *garagetest.Server
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	g := garagetest.New()
	t.Cleanup(g.Close)

	b, err := backend.Factory(context.Background(), logical.TestBackendConfig())
	require.NoError(t, err)
	return &fixture{
		Backend: b.(*backend.Backend),
		Storage: &logical.InmemStorage{},
		Garage:  g,
	}
}

func (f *fixture) writeConfig(t *testing.T, address, token string) *logical.Response {
	t.Helper()
	return f.request(t, logical.UpdateOperation, "config", map[string]interface{}{
		"address": address,
		"token":   token,
	})
}

func (f *fixture) writeConfigOK(t *testing.T) {
	t.Helper()
	resp := f.writeConfig(t, f.Garage.URL(), garagetest.TestBearerToken)
	if resp != nil && resp.IsError() {
		require.FailNow(t, "config write", resp.Error())
	}
}

func (f *fixture) readConfig(t *testing.T) *logical.Response {
	t.Helper()
	return f.request(t, logical.ReadOperation, "config", nil)
}

func (f *fixture) deleteConfig(t *testing.T) *logical.Response {
	t.Helper()
	return f.request(t, logical.DeleteOperation, "config", nil)
}

func (f *fixture) writeRole(t *testing.T, name, bucket string) *logical.Response {
	t.Helper()
	return f.request(t, logical.UpdateOperation, "roles/"+name, map[string]interface{}{
		"bucket": bucket,
		"ttl":    300,
	})
}

func (f *fixture) readRole(t *testing.T, name string) *logical.Response {
	t.Helper()
	return f.request(t, logical.ReadOperation, "roles/"+name, nil)
}

func (f *fixture) deleteRole(t *testing.T, name string) *logical.Response {
	t.Helper()
	return f.request(t, logical.DeleteOperation, "roles/"+name, nil)
}

func (f *fixture) listRoles(t *testing.T) *logical.Response {
	t.Helper()
	return f.request(t, logical.ListOperation, "roles/", nil)
}

func (f *fixture) issueCreds(t *testing.T, role string) *logical.Response {
	t.Helper()
	resp, err := f.Backend.HandleRequest(context.Background(), &logical.Request{
		Operation: logical.ReadOperation,
		Path:      "creds/" + role,
		Storage:   f.Storage,
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.False(t, resp.IsError(), resp.Error())
	require.NotNil(t, resp.Secret)
	return resp
}

func (f *fixture) request(t *testing.T, op logical.Operation, path string, data map[string]interface{}) *logical.Response {
	t.Helper()
	resp, err := f.requestErr(t, op, path, data)
	require.NoError(t, err)
	return resp
}

func (f *fixture) requestErr(t *testing.T, op logical.Operation, path string, data map[string]interface{}) (*logical.Response, error) {
	t.Helper()
	return f.Backend.HandleRequest(context.Background(), &logical.Request{
		Operation: op,
		Path:      path,
		Storage:   f.Storage,
		Data:      data,
	})
}

func setupCredsFixture(t *testing.T) *fixture {
	t.Helper()
	f := newFixture(t)
	f.Garage.AddBucket("my-bucket", "bucket-id-1")
	f.writeConfigOK(t)
	resp := f.writeRole(t, "app", "my-bucket")
	if resp != nil && resp.IsError() {
		require.FailNow(t, "role write", resp.Error())
	}
	return f
}
