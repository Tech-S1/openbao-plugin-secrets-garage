package backend_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ZachTech-HomeLab/vault-plugin-secrets-garage/internal/garage/garagetest"
	"github.com/openbao/openbao/sdk/v2/logical"
	"github.com/stretchr/testify/require"
)

func TestConfigRead(t *testing.T) {
	t.Parallel()

	t.Run("empty", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t)
		resp := f.readConfig(t)
		require.Nil(t, resp)
	})

	t.Run("returns stored values without token", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t)
		write := f.request(t, logical.UpdateOperation, "config", map[string]interface{}{
			"address":     f.Garage.URL(),
			"token":       garagetest.TestBearerToken,
			"default_ttl": 120,
			"max_ttl":     600,
		})
		require.Nil(t, write)

		resp := f.readConfig(t)
		require.NotNil(t, resp)
		require.Equal(t, f.Garage.URL(), resp.Data["address"])
		require.Equal(t, int64(120), resp.Data["default_ttl"])
		require.Equal(t, int64(600), resp.Data["max_ttl"])
		require.NotContains(t, resp.Data, "token")
	})
}

func TestConfigDelete(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	f.writeConfigOK(t)

	resp := f.deleteConfig(t)
	require.Nil(t, resp)

	raw, err := f.Storage.Get(context.Background(), "config")
	require.NoError(t, err)
	require.Nil(t, raw)
	require.Nil(t, f.readConfig(t))
}

func TestConfigWrite(t *testing.T) {
	t.Parallel()

	t.Run("accepts valid token", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t)
		resp := f.writeConfig(t, f.Garage.URL(), garagetest.TestBearerToken)
		if resp != nil {
			require.False(t, resp.IsError(), resp.Error())
		}

		raw, err := f.Storage.Get(context.Background(), "config")
		require.NoError(t, err)
		require.NotNil(t, raw)
	})

	t.Run("preserves token on partial update", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t)
		f.writeConfigOK(t)

		resp := f.request(t, logical.UpdateOperation, "config", map[string]interface{}{
			"address":     f.Garage.URL(),
			"default_ttl": 120,
		})
		require.Nil(t, resp)

		read := f.readConfig(t)
		require.NotNil(t, read)
		require.Equal(t, int64(120), read.Data["default_ttl"])
	})

	t.Run("validation errors", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name       string
			listStatus int
			wantErr    string
		}{
			{
				name:       "rejects unauthorized",
				listStatus: http.StatusUnauthorized,
				wantErr:    "invalid Garage admin token",
			},
			{
				name:       "rejects forbidden",
				listStatus: http.StatusForbidden,
				wantErr:    "invalid Garage admin token",
			},
			{
				name:       "rejects missing API",
				listStatus: http.StatusNotFound,
				wantErr:    "Garage admin API not found",
			},
			{
				name:       "rejects client error",
				listStatus: http.StatusBadRequest,
				wantErr:    "Garage rejected the connection",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				f := newFixture(t)
				f.Garage.ListKeysStatus = tt.listStatus

				resp := f.writeConfig(t, f.Garage.URL(), garagetest.TestBearerToken)
				require.NotNil(t, resp)
				require.True(t, resp.IsError())
				require.Contains(t, resp.Error().Error(), tt.wantErr)

				raw, err := f.Storage.Get(context.Background(), "config")
				require.NoError(t, err)
				require.Nil(t, raw)
			})
		}
	})

	t.Run("field validation", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name    string
			data    map[string]interface{}
			wantErr string
		}{
			{
				name:    "rejects missing address",
				data:    map[string]interface{}{"token": garagetest.TestBearerToken},
				wantErr: "address is required",
			},
			{
				name:    "rejects missing token",
				data:    map[string]interface{}{"address": "http://example.com"},
				wantErr: "token is required",
			},
			{
				name: "rejects default_ttl greater than max_ttl",
				data: map[string]interface{}{
					"address":     "http://example.com",
					"token":       "token",
					"default_ttl": 600,
					"max_ttl":     120,
				},
				wantErr: "default_ttl cannot be greater than max_ttl",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				f := newFixture(t)
				resp := f.request(t, logical.UpdateOperation, "config", tt.data)
				require.NotNil(t, resp)
				require.True(t, resp.IsError())
				require.Contains(t, resp.Error().Error(), tt.wantErr)
			})
		}
	})

	t.Run("rejects unreachable garage", func(t *testing.T) {
		t.Parallel()

		closed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		address := closed.URL
		closed.Close()

		f := newFixture(t)
		resp := f.writeConfig(t, address, garagetest.TestBearerToken)
		require.NotNil(t, resp)
		require.True(t, resp.IsError())
		require.Contains(t, resp.Error().Error(), "cannot reach Garage")
	})

	t.Run("rejects server error", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t)
		f.Garage.ListKeysStatus = http.StatusInternalServerError

		resp := f.writeConfig(t, f.Garage.URL(), garagetest.TestBearerToken)
		require.NotNil(t, resp)
		require.True(t, resp.IsError())
		require.Contains(t, resp.Error().Error(), "failed to validate Garage connection")
	})

	t.Run("rejects corrupt stored config on read", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t)
		require.NoError(t, f.Storage.Put(context.Background(), &logical.StorageEntry{
			Key:   "config",
			Value: []byte("{"),
		}))

		_, err := f.requestErr(t, logical.ReadOperation, "config", nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to decode config")
	})
}
