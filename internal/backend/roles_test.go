package backend_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/openbao/openbao/sdk/v2/logical"
	"github.com/stretchr/testify/require"
)

func TestRoleRead(t *testing.T) {
	t.Parallel()

	t.Run("empty", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t)
		f.writeConfigOK(t)
		require.Nil(t, f.readRole(t, "app"))
	})

	t.Run("returns stored role", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t)
		f.Garage.AddBucket("my-bucket", "bucket-id-1")
		f.writeConfigOK(t)
		f.writeRole(t, "app", "my-bucket")

		resp := f.readRole(t, "app")
		require.NotNil(t, resp)
		require.Equal(t, "my-bucket", resp.Data["bucket"])
		require.Equal(t, int64(300), resp.Data["ttl"])
		require.Equal(t, true, resp.Data["read"])
		require.Equal(t, true, resp.Data["write"])
		require.Equal(t, false, resp.Data["owner"])
	})
}

func TestRoleDelete(t *testing.T) {
	t.Parallel()

	f := newFixture(t)
	f.Garage.AddBucket("my-bucket", "bucket-id-1")
	f.writeConfigOK(t)
	f.writeRole(t, "app", "my-bucket")

	resp := f.deleteRole(t, "app")
	require.Nil(t, resp)
	require.Nil(t, f.readRole(t, "app"))
}

func TestRolesList(t *testing.T) {
	t.Parallel()

	t.Run("empty", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t)
		f.writeConfigOK(t)

		resp := f.listRoles(t)
		require.NotNil(t, resp)
		require.Nil(t, resp.Data["keys"])
	})

	t.Run("lists role names", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t)
		f.Garage.AddBucket("my-bucket", "bucket-id-1")
		f.writeConfigOK(t)
		f.writeRole(t, "app", "my-bucket")
		f.writeRole(t, "worker", "my-bucket")

		resp := f.listRoles(t)
		require.NotNil(t, resp)
		require.ElementsMatch(t, []string{"app", "worker"}, resp.Data["keys"])
	})
}

func TestRoleWrite(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		setup     func(*fixture)
		bucket    string
		data      map[string]interface{}
		wantError string
	}{
		{
			name: "rejects empty bucket",
			setup: func(f *fixture) {
				f.Garage.AddBucket("my-bucket", "bucket-id-1")
			},
			data:      map[string]interface{}{"ttl": 300},
			wantError: "bucket is required",
		},
		{
			name:      "rejects missing bucket",
			bucket:    "missing-bucket",
			wantError: `bucket "missing-bucket" does not exist`,
		},
		{
			name: "accepts existing bucket",
			setup: func(f *fixture) {
				f.Garage.AddBucket("my-bucket", "bucket-id-1")
			},
			bucket: "my-bucket",
		},
		{
			name:      "rejects when garage is not configured",
			bucket:    "my-bucket",
			wantError: "garage is not configured",
		},
		{
			name: "rejects ttl greater than max_ttl",
			setup: func(f *fixture) {
				f.Garage.AddBucket("my-bucket", "bucket-id-1")
			},
			bucket: "my-bucket",
			data: map[string]interface{}{
				"bucket":  "my-bucket",
				"ttl":     600,
				"max_ttl": 120,
			},
			wantError: "ttl cannot be greater than max_ttl",
		},
		{
			name: "rejects bucket validation error",
			setup: func(f *fixture) {
				f.Garage.AddBucket("my-bucket", "bucket-id-1")
				f.Garage.GetBucketInfoStatus = http.StatusBadRequest
			},
			bucket:    "my-bucket",
			wantError: `failed to validate bucket "my-bucket"`,
		},
		{
			name: "rejects bucket server error",
			setup: func(f *fixture) {
				f.Garage.AddBucket("my-bucket", "bucket-id-1")
				f.Garage.GetBucketInfoStatus = http.StatusInternalServerError
			},
			bucket:    "my-bucket",
			wantError: "GetBucketInfo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t)
			if tt.setup != nil {
				tt.setup(f)
			}
			if tt.wantError != "garage is not configured" {
				f.writeConfigOK(t)
			}

			var resp *logical.Response
			var err error
			if tt.data != nil {
				resp, err = f.requestErr(t, logical.UpdateOperation, "roles/app", tt.data)
			} else {
				resp, err = f.requestErr(t, logical.UpdateOperation, "roles/app", map[string]interface{}{
					"bucket": tt.bucket,
					"ttl":    300,
				})
			}
			if tt.wantError == "GetBucketInfo" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.wantError)
				return
			}
			require.NoError(t, err)
			if tt.wantError != "" {
				require.NotNil(t, resp)
				require.True(t, resp.IsError())
				require.Contains(t, resp.Error().Error(), tt.wantError)
				return
			}
			if resp != nil {
				require.False(t, resp.IsError(), resp.Error())
			}
		})
	}

	t.Run("updates existing role", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t)
		f.Garage.AddBucket("my-bucket", "bucket-id-1")
		f.writeConfigOK(t)
		f.writeRole(t, "app", "my-bucket")

		resp := f.request(t, logical.UpdateOperation, "roles/app", map[string]interface{}{
			"bucket": "my-bucket",
			"owner":  true,
			"write":  false,
		})
		require.Nil(t, resp)

		read := f.readRole(t, "app")
		require.Equal(t, true, read.Data["owner"])
		require.Equal(t, false, read.Data["write"])
	})

	t.Run("rejects corrupt stored role on read", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t)
		f.writeConfigOK(t)
		require.NoError(t, f.Storage.Put(context.Background(), &logical.StorageEntry{
			Key:   "roles/app",
			Value: []byte("{"),
		}))

		_, err := f.requestErr(t, logical.ReadOperation, "roles/app", nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to decode role app")
	})
}
