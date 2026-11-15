package backend_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/ZachTech-HomeLab/vault-plugin-secrets-garage/internal/backend"
	"github.com/ZachTech-HomeLab/vault-plugin-secrets-garage/internal/garage/garagetest"
	"github.com/openbao/openbao/sdk/v2/logical"
	"github.com/stretchr/testify/require"
)

func TestCredsIssue(t *testing.T) {
	t.Parallel()

	t.Run("happy path", func(t *testing.T) {
		t.Parallel()

		f := setupCredsFixture(t)
		resp := f.issueCreds(t, "app")

		require.Equal(t, garagetest.TestAccessKeyID, resp.Secret.InternalData["access_key_id"])
		require.Equal(t, int64(300), resp.Secret.InternalData["ttl"])
		require.Equal(t, 1, f.Garage.CreateKeyCalls)
		require.Equal(t, 1, f.Garage.AllowBucketKeyCalls)
	})

	t.Run("rolls back key on AllowBucketKey failure", func(t *testing.T) {
		t.Parallel()

		f := setupCredsFixture(t)
		f.Garage.AllowBucketKeyStatus = 500

		_, err := f.Backend.HandleRequest(context.Background(), &logical.Request{
			Operation: logical.ReadOperation,
			Path:      "creds/app",
			Storage:   f.Storage,
		})
		require.Error(t, err)
		require.Equal(t, 1, f.Garage.CreateKeyCalls)
		require.EqualValues(t, 1, f.Garage.DeleteKeyCalls.Load())
	})

	t.Run("rejects missing role", func(t *testing.T) {
		t.Parallel()

		f := setupCredsFixture(t)
		resp := f.request(t, logical.ReadOperation, "creds/missing", nil)
		require.NotNil(t, resp)
		require.True(t, resp.IsError())
		require.Contains(t, resp.Error().Error(), `role "missing" does not exist`)
	})

	t.Run("uses config default ttl", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t)
		f.Garage.AddBucket("my-bucket", "bucket-id-1")
		f.request(t, logical.UpdateOperation, "config", map[string]interface{}{
			"address":     f.Garage.URL(),
			"token":       garagetest.TestBearerToken,
			"default_ttl": 120,
		})
		f.request(t, logical.UpdateOperation, "roles/app", map[string]interface{}{
			"bucket": "my-bucket",
		})

		resp := f.issueCreds(t, "app")
		require.Equal(t, int64(120), resp.Secret.InternalData["ttl"])
	})

	t.Run("caps ttl to config max", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t)
		f.Garage.AddBucket("my-bucket", "bucket-id-1")
		f.request(t, logical.UpdateOperation, "config", map[string]interface{}{
			"address":     f.Garage.URL(),
			"token":       garagetest.TestBearerToken,
			"default_ttl": 60,
			"max_ttl":     120,
		})
		f.request(t, logical.UpdateOperation, "roles/app", map[string]interface{}{
			"bucket": "my-bucket",
			"ttl":    600,
		})

		resp := f.issueCreds(t, "app")
		require.Equal(t, int64(120), resp.Secret.InternalData["ttl"])
	})

	t.Run("rejects when garage is not configured", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t)
		require.NoError(t, f.Storage.Put(context.Background(), &logical.StorageEntry{
			Key:   "roles/app",
			Value: []byte(`{"bucket":"my-bucket","read":true,"write":true}`),
		}))

		resp, err := f.requestErr(t, logical.ReadOperation, "creds/app", nil)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.True(t, resp.IsError())
		require.Contains(t, resp.Error().Error(), "garage is not configured")
	})

	t.Run("skips bucket grant when role has no bucket", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t)
		f.writeConfigOK(t)
		require.NoError(t, f.Storage.Put(context.Background(), &logical.StorageEntry{
			Key:   "roles/app",
			Value: []byte(`{"bucket":"","read":true,"write":true}`),
		}))

		resp := f.issueCreds(t, "app")
		require.NotNil(t, resp.Secret)
		require.Equal(t, 0, f.Garage.AllowBucketKeyCalls)
	})

	t.Run("rolls back key on GetBucketInfo failure", func(t *testing.T) {
		t.Parallel()

		f := setupCredsFixture(t)
		f.Garage.GetBucketInfoStatus = http.StatusInternalServerError

		_, err := f.Backend.HandleRequest(context.Background(), &logical.Request{
			Operation: logical.ReadOperation,
			Path:      "creds/app",
			Storage:   f.Storage,
		})
		require.Error(t, err)
		require.Equal(t, 1, f.Garage.CreateKeyCalls)
		require.EqualValues(t, 1, f.Garage.DeleteKeyCalls.Load())
	})

	t.Run("rejects CreateKey failure", func(t *testing.T) {
		t.Parallel()

		f := setupCredsFixture(t)
		f.Garage.CreateKeyStatus = http.StatusInternalServerError

		_, err := f.Backend.HandleRequest(context.Background(), &logical.Request{
			Operation: logical.ReadOperation,
			Path:      "creds/app",
			Storage:   f.Storage,
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "CreateKey")
	})
}

func TestSecretRevoke(t *testing.T) {
	t.Parallel()

	t.Run("deletes key", func(t *testing.T) {
		t.Parallel()

		f := setupCredsFixture(t)
		issue := f.issueCreds(t, "app")

		_, err := f.Backend.HandleRequest(context.Background(), &logical.Request{
			Operation: logical.RevokeOperation,
			Storage:   f.Storage,
			Secret:    issue.Secret,
		})
		require.NoError(t, err)
		require.EqualValues(t, 1, f.Garage.DeleteKeyCalls.Load())
	})

	t.Run("retries transient errors", func(t *testing.T) {
		t.Parallel()

		f := setupCredsFixture(t)
		f.Garage.DeleteKeyFailUntil.Store(2)
		issue := f.issueCreds(t, "app")

		_, err := f.Backend.HandleRequest(context.Background(), &logical.Request{
			Operation: logical.RevokeOperation,
			Storage:   f.Storage,
			Secret:    issue.Secret,
		})
		require.NoError(t, err)
		require.EqualValues(t, 3, f.Garage.DeleteKeyCalls.Load())
	})
}

func TestSecretRenewUsesSnapshotAfterRoleDelete(t *testing.T) {
	t.Parallel()

	f := setupCredsFixture(t)
	issue := f.issueCreds(t, "app")
	require.NoError(t, f.Storage.Delete(context.Background(), "roles/app"))

	renew, err := f.Backend.HandleRequest(context.Background(), &logical.Request{
		Operation: logical.RenewOperation,
		Storage:   f.Storage,
		Secret:    issue.Secret,
	})
	require.NoError(t, err)
	require.NotNil(t, renew)
	require.NotNil(t, renew.Secret)
	require.Equal(t, 300*time.Second, renew.Secret.TTL)
	require.Equal(t, 15*time.Minute, renew.Secret.MaxTTL)
}

func TestSecretRenewCapsToRemainingMaxTTL(t *testing.T) {
	t.Parallel()

	f := setupCredsFixture(t)
	issue := f.issueCreds(t, "app")
	issue.Secret.IssueTime = time.Now().UTC().Add(-14 * time.Minute)
	issue.Secret.Increment = 10 * time.Minute

	before := time.Now().UTC()
	renew, err := f.Backend.HandleRequest(context.Background(), &logical.Request{
		Operation: logical.RenewOperation,
		Storage:   f.Storage,
		Secret:    issue.Secret,
	})
	require.NoError(t, err)
	require.NotNil(t, renew)
	require.LessOrEqual(t, renew.Secret.TTL, 75*time.Second)
	require.Greater(t, renew.Secret.TTL, time.Duration(0))

	exp := f.Garage.LastUpdateKeyExp
	require.False(t, exp.IsZero())
	require.WithinDuration(t, before.Add(renew.Secret.TTL), exp, 2*time.Second)
}

func TestCredsCapsTTLToSystemMax(t *testing.T) {
	t.Parallel()

	g := garagetest.New()
	t.Cleanup(g.Close)
	g.AddBucket("my-bucket", "bucket-id-1")

	cfg := logical.TestBackendConfig()
	cfg.System = &logical.StaticSystemView{
		DefaultLeaseTTLVal: time.Hour,
		MaxLeaseTTLVal:     90 * time.Second,
	}
	b, err := backend.Factory(context.Background(), cfg)
	require.NoError(t, err)

	f := &fixture{
		Backend: b.(*backend.Backend),
		Storage: &logical.InmemStorage{},
		Garage:  g,
	}
	f.writeConfigOK(t)
	resp := f.writeRole(t, "app", "my-bucket")
	if resp != nil && resp.IsError() {
		require.FailNow(t, "role write", resp.Error())
	}

	issued := f.issueCreds(t, "app")
	require.Equal(t, int64(90), issued.Secret.InternalData["ttl"])
	require.Equal(t, int64(90), issued.Secret.InternalData["max_ttl"])
	require.WithinDuration(t, time.Now().UTC().Add(90*time.Second), f.Garage.LastCreateKeyExp, 2*time.Second)
}

func TestCredsMapsMissingBucketToErrorResponse(t *testing.T) {
	t.Parallel()

	f := setupCredsFixture(t)
	require.NoError(t, f.Storage.Put(context.Background(), &logical.StorageEntry{
		Key:   "roles/app",
		Value: []byte(`{"bucket":"gone-bucket","read":true,"write":true,"ttl":300000000000}`),
	}))

	resp, err := f.requestErr(t, logical.ReadOperation, "creds/app", nil)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.True(t, resp.IsError())
	require.Contains(t, resp.Error().Error(), `bucket "gone-bucket" does not exist`)
}

func TestCredsMapsClientErrorToErrorResponse(t *testing.T) {
	t.Parallel()

	f := setupCredsFixture(t)
	f.Garage.CreateKeyStatus = http.StatusBadRequest

	resp, err := f.requestErr(t, logical.ReadOperation, "creds/app", nil)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.True(t, resp.IsError())
	require.Contains(t, resp.Error().Error(), "failed to provision Garage credentials")
}

func TestProvisionRollbackLogsDeleteFailure(t *testing.T) {
	t.Parallel()

	f := setupCredsFixture(t)
	f.Garage.AllowBucketKeyStatus = 500
	f.Garage.DeleteKeyStatus = http.StatusInternalServerError

	_, err := f.Backend.HandleRequest(context.Background(), &logical.Request{
		Operation: logical.ReadOperation,
		Path:      "creds/app",
		Storage:   f.Storage,
	})
	require.Error(t, err)
	require.Equal(t, 1, f.Garage.CreateKeyCalls)
	require.EqualValues(t, 1, f.Garage.DeleteKeyCalls.Load())
}

func TestSecretRenewRejectsInvalidSnapshot(t *testing.T) {
	t.Parallel()

	f := setupCredsFixture(t)
	issue := f.issueCreds(t, "app")
	issue.Secret.InternalData["ttl"] = "not-a-duration"

	_, err := f.Backend.HandleRequest(context.Background(), &logical.Request{
		Operation: logical.RenewOperation,
		Storage:   f.Storage,
		Secret:    issue.Secret,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "secret has invalid ttl")
}

func TestSecretRenewRejectsMissingConfig(t *testing.T) {
	t.Parallel()

	f := setupCredsFixture(t)
	issue := f.issueCreds(t, "app")
	require.NoError(t, f.Storage.Delete(context.Background(), "config"))

	_, err := f.Backend.HandleRequest(context.Background(), &logical.Request{
		Operation: logical.RenewOperation,
		Storage:   f.Storage,
		Secret:    issue.Secret,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "garage is not configured")
}

func TestSecretRenewRejectsMissingMaxTTL(t *testing.T) {
	t.Parallel()

	f := setupCredsFixture(t)
	issue := f.issueCreds(t, "app")
	delete(issue.Secret.InternalData, "max_ttl")

	_, err := f.Backend.HandleRequest(context.Background(), &logical.Request{
		Operation: logical.RenewOperation,
		Storage:   f.Storage,
		Secret:    issue.Secret,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "secret is missing max_ttl")
}

func TestSecretRenewRejectsUpdateKeyFailure(t *testing.T) {
	t.Parallel()

	f := setupCredsFixture(t)
	issue := f.issueCreds(t, "app")
	f.Garage.UpdateKeyStatus = http.StatusInternalServerError

	_, err := f.Backend.HandleRequest(context.Background(), &logical.Request{
		Operation: logical.RenewOperation,
		Storage:   f.Storage,
		Secret:    issue.Secret,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "UpdateKey")
}

func TestSecretRenewAcceptsAlternateSnapshotTypes(t *testing.T) {
	t.Parallel()

	f := setupCredsFixture(t)
	issue := f.issueCreds(t, "app")
	issue.Secret.InternalData["ttl"] = 300
	issue.Secret.InternalData["max_ttl"] = float64(900)

	renew, err := f.Backend.HandleRequest(context.Background(), &logical.Request{
		Operation: logical.RenewOperation,
		Storage:   f.Storage,
		Secret:    issue.Secret,
	})
	require.NoError(t, err)
	require.Equal(t, 300*time.Second, renew.Secret.TTL)
	require.Equal(t, 900*time.Second, renew.Secret.MaxTTL)
}

func TestSecretRevokeRejectsMissingConfig(t *testing.T) {
	t.Parallel()

	f := setupCredsFixture(t)
	issue := f.issueCreds(t, "app")
	require.NoError(t, f.Storage.Delete(context.Background(), "config"))

	_, err := f.Backend.HandleRequest(context.Background(), &logical.Request{
		Operation: logical.RevokeOperation,
		Storage:   f.Storage,
		Secret:    issue.Secret,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "garage is not configured")
}

func TestSecretRevokeRejectsMissingAccessKey(t *testing.T) {
	t.Parallel()

	f := setupCredsFixture(t)
	issue := f.issueCreds(t, "app")
	delete(issue.Secret.InternalData, "access_key_id")

	_, err := f.Backend.HandleRequest(context.Background(), &logical.Request{
		Operation: logical.RevokeOperation,
		Storage:   f.Storage,
		Secret:    issue.Secret,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "secret is missing access_key_id")
}
