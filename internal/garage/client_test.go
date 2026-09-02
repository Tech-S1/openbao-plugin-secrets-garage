package garage_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ZachTech-HomeLab/vault-plugin-secrets-garage/internal/garage"
	"github.com/ZachTech-HomeLab/vault-plugin-secrets-garage/internal/garage/garagetest"
)

func testClient(t *testing.T, srv *garagetest.Server) *garage.HTTPClient {
	t.Helper()
	return garage.New(srv.URL(), garagetest.TestBearerToken, garage.WithHTTPClient(srv.HTTP.Client()))
}

func TestNewTrimsTrailingSlash(t *testing.T) {
	t.Parallel()

	srv := garagetest.New()
	t.Cleanup(srv.Close)

	client := garage.New(srv.URL()+"/", garagetest.TestBearerToken, garage.WithHTTPClient(srv.HTTP.Client()))
	if err := client.ValidateConnection(context.Background()); err != nil {
		t.Fatalf("ValidateConnection: %v", err)
	}
}

func TestValidateConnection(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		srv := garagetest.New()
		t.Cleanup(srv.Close)
		if err := testClient(t, srv).ValidateConnection(context.Background()); err != nil {
			t.Fatalf("ValidateConnection: %v", err)
		}
	})

	t.Run("unauthorized", func(t *testing.T) {
		t.Parallel()
		srv := garagetest.New()
		t.Cleanup(srv.Close)
		client := garage.New(srv.URL(), "wrong-token", garage.WithHTTPClient(srv.HTTP.Client()))
		err := client.ValidateConnection(context.Background())
		assertAPIError(t, err, http.StatusUnauthorized)
	})

	t.Run("server error", func(t *testing.T) {
		t.Parallel()
		srv := garagetest.New()
		t.Cleanup(srv.Close)
		srv.ListKeysStatus = http.StatusInternalServerError
		err := testClient(t, srv).ValidateConnection(context.Background())
		assertAPIError(t, err, http.StatusInternalServerError)
	})
}

func TestCreateKey(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		srv := garagetest.New()
		t.Cleanup(srv.Close)

		exp := time.Now().UTC().Add(time.Hour)
		key, err := testClient(t, srv).CreateKey(context.Background(), "my-key", exp)
		if err != nil {
			t.Fatalf("CreateKey: %v", err)
		}
		if key.AccessKeyID != garagetest.TestAccessKeyID {
			t.Fatalf("access key id = %q", key.AccessKeyID)
		}
		if key.SecretAccessKey != garagetest.TestSecretKey {
			t.Fatalf("secret = %q", key.SecretAccessKey)
		}
		if srv.CreateKeyCalls != 1 {
			t.Fatalf("CreateKeyCalls = %d", srv.CreateKeyCalls)
		}
	})

	t.Run("missing access key id", func(t *testing.T) {
		t.Parallel()
		client := garage.New(mockServer(t, func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]string{"secretAccessKey": "x"})
		}).URL, "token", garage.WithHTTPClient(http.DefaultClient))

		_, err := client.CreateKey(context.Background(), "my-key", time.Now())
		if err == nil || !strings.Contains(err.Error(), "no accessKeyId") {
			t.Fatalf("expected missing accessKeyId error, got %v", err)
		}
	})

	t.Run("missing secret", func(t *testing.T) {
		t.Parallel()
		client := garage.New(mockServer(t, func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]string{"accessKeyId": "GK123"})
		}).URL, "token", garage.WithHTTPClient(http.DefaultClient))

		_, err := client.CreateKey(context.Background(), "my-key", time.Now())
		if err == nil || !strings.Contains(err.Error(), "no secretAccessKey") {
			t.Fatalf("expected missing secretAccessKey error, got %v", err)
		}
	})

	t.Run("api error", func(t *testing.T) {
		t.Parallel()
		client := garage.New(mockServer(t, func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "quota exceeded", http.StatusForbidden)
		}).URL, "token", garage.WithHTTPClient(http.DefaultClient))

		_, err := client.CreateKey(context.Background(), "my-key", time.Now())
		assertAPIError(t, err, http.StatusForbidden)
	})
}

func TestUpdateKey(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		srv := garagetest.New()
		t.Cleanup(srv.Close)

		exp := time.Now().UTC().Add(2 * time.Hour)
		if err := testClient(t, srv).UpdateKey(context.Background(), "key-id", exp); err != nil {
			t.Fatalf("UpdateKey: %v", err)
		}
	})

	t.Run("api error", func(t *testing.T) {
		t.Parallel()
		client := garage.New(mockServer(t, func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "not found", http.StatusNotFound)
		}).URL, "token", garage.WithHTTPClient(http.DefaultClient))

		err := client.UpdateKey(context.Background(), "missing", time.Now())
		assertAPIError(t, err, http.StatusNotFound)
	})
}

func TestDeleteKey(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		srv := garagetest.New()
		t.Cleanup(srv.Close)

		if err := testClient(t, srv).DeleteKey(context.Background(), "key-id"); err != nil {
			t.Fatalf("DeleteKey: %v", err)
		}
		if srv.DeleteKeyCalls.Load() != 1 {
			t.Fatalf("DeleteKeyCalls = %d", srv.DeleteKeyCalls.Load())
		}
	})

	t.Run("not found is success", func(t *testing.T) {
		t.Parallel()
		client := garage.New(mockServer(t, func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "not found", http.StatusNotFound)
		}).URL, "token", garage.WithHTTPClient(http.DefaultClient))

		if err := client.DeleteKey(context.Background(), "gone"); err != nil {
			t.Fatalf("DeleteKey: %v", err)
		}
	})

	t.Run("other api error", func(t *testing.T) {
		t.Parallel()
		client := garage.New(mockServer(t, func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "bad gateway", http.StatusBadGateway)
		}).URL, "token", garage.WithHTTPClient(http.DefaultClient))

		err := client.DeleteKey(context.Background(), "key-id")
		assertAPIError(t, err, http.StatusBadGateway)
	})
}

func TestDeleteKeyWithRetry(t *testing.T) {
	t.Parallel()

	t.Run("succeeds after transient failure", func(t *testing.T) {
		t.Parallel()
		attempts := 0
		srv := mockServer(t, func(w http.ResponseWriter, r *http.Request) {
			attempts++
			if attempts < 3 {
				http.Error(w, "server error", http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusOK)
		})
		client := garage.New(srv.URL, "token", garage.WithHTTPClient(srv.Client()))

		if err := client.DeleteKeyWithRetry(context.Background(), "key-id"); err != nil {
			t.Fatalf("DeleteKeyWithRetry: %v", err)
		}
		if attempts != 3 {
			t.Fatalf("attempts = %d, want 3", attempts)
		}
	})

	t.Run("does not retry client errors", func(t *testing.T) {
		t.Parallel()
		attempts := 0
		srv := mockServer(t, func(w http.ResponseWriter, r *http.Request) {
			attempts++
			http.Error(w, "bad request", http.StatusBadRequest)
		})
		client := garage.New(srv.URL, "token", garage.WithHTTPClient(srv.Client()))

		err := client.DeleteKeyWithRetry(context.Background(), "key-id")
		if err == nil {
			t.Fatal("expected error")
		}
		if attempts != 1 {
			t.Fatalf("attempts = %d, want 1", attempts)
		}
	})

	t.Run("respects context cancellation", func(t *testing.T) {
		t.Parallel()
		srv := mockServer(t, func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "server error", http.StatusInternalServerError)
		})
		client := garage.New(srv.URL, "token", garage.WithHTTPClient(srv.Client()))

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err := client.DeleteKeyWithRetry(ctx, "key-id")
		if err != context.Canceled {
			t.Fatalf("err = %v, want context.Canceled", err)
		}
	})
}

func TestGetBucketInfo(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		srv := garagetest.New()
		t.Cleanup(srv.Close)
		srv.AddBucket("photos", "bucket-abc")

		bucket, err := testClient(t, srv).GetBucketInfo(context.Background(), "photos")
		if err != nil {
			t.Fatalf("GetBucketInfo: %v", err)
		}
		if bucket.ID != "bucket-abc" {
			t.Fatalf("id = %q", bucket.ID)
		}
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()
		srv := garagetest.New()
		t.Cleanup(srv.Close)

		_, err := testClient(t, srv).GetBucketInfo(context.Background(), "missing")
		assertAPIError(t, err, http.StatusNotFound)
	})

	t.Run("empty id in response", func(t *testing.T) {
		t.Parallel()
		client := garage.New(mockServer(t, func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]string{"id": ""})
		}).URL, "token", garage.WithHTTPClient(http.DefaultClient))

		_, err := client.GetBucketInfo(context.Background(), "photos")
		if err == nil || !strings.Contains(err.Error(), "returned no id") {
			t.Fatalf("expected empty id error, got %v", err)
		}
	})
}

func TestAllowBucketKey(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		srv := garagetest.New()
		t.Cleanup(srv.Close)

		perms := garage.Permissions{Read: true, Write: false, Owner: false}
		if err := testClient(t, srv).AllowBucketKey(context.Background(), "bucket-id", "key-id", perms); err != nil {
			t.Fatalf("AllowBucketKey: %v", err)
		}
		if srv.AllowBucketKeyCalls != 1 {
			t.Fatalf("AllowBucketKeyCalls = %d", srv.AllowBucketKeyCalls)
		}
	})

	t.Run("api error", func(t *testing.T) {
		t.Parallel()
		srv := garagetest.New()
		t.Cleanup(srv.Close)
		srv.AllowBucketKeyStatus = http.StatusInternalServerError

		err := testClient(t, srv).AllowBucketKey(context.Background(), "bucket-id", "key-id", garage.Permissions{})
		assertAPIError(t, err, http.StatusInternalServerError)
	})

	t.Run("sends permissions in body", func(t *testing.T) {
		t.Parallel()
		var body map[string]any
		client := garage.New(mockServer(t, func(w http.ResponseWriter, r *http.Request) {
			raw, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(raw, &body)
			w.WriteHeader(http.StatusOK)
		}).URL, "token", garage.WithHTTPClient(http.DefaultClient))

		perms := garage.Permissions{Read: true, Write: true, Owner: false}
		if err := client.AllowBucketKey(context.Background(), "b1", "k1", perms); err != nil {
			t.Fatalf("AllowBucketKey: %v", err)
		}
		permsRaw, ok := body["permissions"].(map[string]any)
		if !ok {
			t.Fatalf("permissions missing from body: %#v", body)
		}
		if permsRaw["read"] != true || permsRaw["write"] != true || permsRaw["owner"] != false {
			t.Fatalf("permissions = %#v", permsRaw)
		}
		if body["bucketId"] != "b1" || body["accessKeyId"] != "k1" {
			t.Fatalf("body = %#v", body)
		}
	})
}

func mockServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	return httptest.NewServer(handler)
}

func assertAPIError(t *testing.T, err error, wantStatus int) {
	t.Helper()
	if err == nil {
		t.Fatal("expected error")
	}
	apiErr, ok := garage.AsAPIError(err)
	if !ok {
		t.Fatalf("expected APIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != wantStatus {
		t.Fatalf("status = %d, want %d", apiErr.StatusCode, wantStatus)
	}
}
