package backend

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/ZachTech-HomeLab/vault-plugin-secrets-garage/internal/garage"
	"github.com/openbao/openbao/sdk/v2/framework"
	"github.com/openbao/openbao/sdk/v2/logical"
	"github.com/stretchr/testify/require"
)

func TestConfigExistence(t *testing.T) {
	t.Parallel()

	b := New()
	storage := &logical.InmemStorage{}
	req := &logical.Request{Storage: storage}

	exists, err := b.configExistence(context.Background(), req, nil)
	require.NoError(t, err)
	require.False(t, exists)

	entry, err := logical.StorageEntryJSON(storageConfigKey, &Config{
		Address: "http://garage",
		Token:   "token",
	})
	require.NoError(t, err)
	require.NoError(t, storage.Put(context.Background(), entry))

	exists, err = b.configExistence(context.Background(), req, nil)
	require.NoError(t, err)
	require.True(t, exists)
}

func TestRoleExistence(t *testing.T) {
	t.Parallel()

	b := New()
	storage := &logical.InmemStorage{}
	req := &logical.Request{Storage: storage}
	fd := &framework.FieldData{
		Raw:    map[string]interface{}{"name": "app"},
		Schema: b.pathRoles().Fields,
	}

	exists, err := b.roleExistence(context.Background(), req, fd)
	require.NoError(t, err)
	require.False(t, exists)

	entry, err := logical.StorageEntryJSON(storageRolesPrefix+"app", &Role{Bucket: "my-bucket"})
	require.NoError(t, err)
	require.NoError(t, storage.Put(context.Background(), entry))

	exists, err = b.roleExistence(context.Background(), req, fd)
	require.NoError(t, err)
	require.True(t, exists)
}

func TestSnapshotDuration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		value   interface{}
		want    time.Duration
		wantErr string
	}{
		{name: "int64", value: int64(120), want: 120 * time.Second},
		{name: "int", value: 90, want: 90 * time.Second},
		{name: "float64", value: float64(60), want: 60 * time.Second},
		{name: "json number", value: json.Number("45"), want: 45 * time.Second},
		{name: "missing", wantErr: "secret is missing ttl"},
		{name: "invalid type", value: "bad", wantErr: "secret has invalid ttl"},
		{name: "invalid json number", value: json.Number("bad"), wantErr: "secret has invalid ttl"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			data := map[string]interface{}{}
			if tt.value != nil {
				data["ttl"] = tt.value
			}
			got, err := snapshotDuration(data, "ttl")
			if tt.wantErr != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestResolveTTL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		roleTTL time.Duration
		roleMax time.Duration
		cfgDef  time.Duration
		cfgMax  time.Duration
		wantTTL time.Duration
		wantMax time.Duration
	}{
		{
			name:    "uses role ttl",
			roleTTL: 2 * time.Minute,
			cfgDef:  5 * time.Minute,
			wantTTL: 2 * time.Minute,
		},
		{
			name:    "falls back to config default",
			cfgDef:  2 * time.Minute,
			wantTTL: 2 * time.Minute,
		},
		{
			name:    "falls back to hardcoded default",
			wantTTL: defaultTTL,
		},
		{
			name:    "caps ttl to max",
			roleTTL: 10 * time.Minute,
			cfgMax:  2 * time.Minute,
			wantTTL: 2 * time.Minute,
			wantMax: 2 * time.Minute,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ttl, maxTTL := resolveTTL(tt.roleTTL, tt.roleMax, tt.cfgDef, tt.cfgMax)
			require.Equal(t, tt.wantTTL, ttl)
			require.Equal(t, tt.wantMax, maxTTL)
		})
	}
}

func TestConfigValidationError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		err     error
		wantErr string
	}{
		{
			name:    "nil",
			err:     nil,
			wantErr: "",
		},
		{
			name:    "server error",
			err:     &garage.APIError{StatusCode: http.StatusInternalServerError, Message: "boom"},
			wantErr: "failed to validate Garage connection",
		},
		{
			name:    "network error",
			err:     &net.OpError{Op: "dial", Err: errors.New("refused")},
			wantErr: "cannot reach Garage",
		},
		{
			name:    "deadline exceeded",
			err:     context.DeadlineExceeded,
			wantErr: "cannot reach Garage",
		},
		{
			name:    "generic error",
			err:     fmt.Errorf("unexpected"),
			wantErr: "failed to validate Garage connection",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			resp := configValidationError(tt.err)
			if tt.wantErr == "" {
				require.Nil(t, resp)
				return
			}
			require.NotNil(t, resp)
			require.Contains(t, resp.Error().Error(), tt.wantErr)
		})
	}
}

func TestBucketValidationError(t *testing.T) {
	t.Parallel()

	t.Run("server error", func(t *testing.T) {
		t.Parallel()

		resp, err := bucketValidationError(&garage.APIError{
			StatusCode: http.StatusInternalServerError,
			Message:    "boom",
		}, "my-bucket")
		require.Nil(t, resp)
		require.Error(t, err)
		require.Contains(t, err.Error(), "GetBucketInfo")
	})
}

func TestGetConfigRejectsInvalidJSON(t *testing.T) {
	t.Parallel()

	b := New()
	storage := &logical.InmemStorage{}
	require.NoError(t, storage.Put(context.Background(), &logical.StorageEntry{
		Key:   storageConfigKey,
		Value: []byte("{"),
	}))

	_, err := b.getConfig(context.Background(), storage)
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to decode config")
}

func TestGetRoleRejectsInvalidJSON(t *testing.T) {
	t.Parallel()

	b := New()
	storage := &logical.InmemStorage{}
	require.NoError(t, storage.Put(context.Background(), &logical.StorageEntry{
		Key:   storageRolesPrefix + "app",
		Value: []byte("{"),
	}))

	_, err := b.getRole(context.Background(), storage, "app")
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to decode role app")
}
