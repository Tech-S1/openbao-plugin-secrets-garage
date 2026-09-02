package garage

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const defaultTimeout = 15 * time.Second

type Client interface {
	ValidateConnection(ctx context.Context) error
	CreateKey(ctx context.Context, name string, expiration time.Time) (*Key, error)
	UpdateKey(ctx context.Context, id string, expiration time.Time) error
	DeleteKey(ctx context.Context, id string) error
	DeleteKeyWithRetry(ctx context.Context, id string) error
	GetBucketInfo(ctx context.Context, alias string) (*Bucket, error)
	AllowBucketKey(ctx context.Context, bucketID, accessKeyID string, perms Permissions) error
}

type HTTPClient struct {
	address string
	token   string
	http    *http.Client
}

type Option func(*HTTPClient)

func WithHTTPClient(c *http.Client) Option {
	return func(client *HTTPClient) {
		client.http = c
	}
}

func WithTimeout(d time.Duration) Option {
	return func(client *HTTPClient) {
		client.http.Timeout = d
	}
}

func New(address, token string, opts ...Option) *HTTPClient {
	c := &HTTPClient{
		address: strings.TrimRight(address, "/"),
		token:   token,
		http:    &http.Client{Timeout: defaultTimeout},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

func (c *HTTPClient) ValidateConnection(ctx context.Context) error {
	var keys []keyInfo
	return c.do(ctx, http.MethodGet, "/v2/ListKeys", nil, nil, &keys)
}

func (c *HTTPClient) CreateKey(ctx context.Context, name string, expiration time.Time) (*Key, error) {
	req := createKeyRequest{
		Name:         name,
		Expiration:   expiration.UTC(),
		NeverExpires: false,
	}
	var out keyInfo
	if err := c.do(ctx, http.MethodPost, "/v2/CreateKey", nil, req, &out); err != nil {
		return nil, err
	}
	if out.AccessKeyID == "" {
		return nil, fmt.Errorf("garage CreateKey returned no accessKeyId")
	}
	if out.SecretAccessKey == nil || *out.SecretAccessKey == "" {
		return nil, fmt.Errorf("garage CreateKey returned no secretAccessKey")
	}
	return &Key{
		AccessKeyID:     out.AccessKeyID,
		SecretAccessKey: *out.SecretAccessKey,
	}, nil
}

func (c *HTTPClient) UpdateKey(ctx context.Context, id string, expiration time.Time) error {
	req := updateKeyRequest{
		Expiration:   expiration.UTC(),
		NeverExpires: false,
	}
	query := url.Values{}
	query.Set("id", id)
	return c.do(ctx, http.MethodPost, "/v2/UpdateKey", query, req, nil)
}

func (c *HTTPClient) DeleteKey(ctx context.Context, id string) error {
	query := url.Values{}
	query.Set("id", id)
	err := c.do(ctx, http.MethodPost, "/v2/DeleteKey", query, nil, nil)
	if err != nil {
		var apiErr *APIError
		if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound {
			return nil
		}
	}
	return err
}

var deleteKeyBackoff = []time.Duration{
	100 * time.Millisecond,
	500 * time.Millisecond,
	2 * time.Second,
}

func (c *HTTPClient) DeleteKeyWithRetry(ctx context.Context, id string) error {
	var lastErr error
	for attempt := 0; attempt <= len(deleteKeyBackoff); attempt++ {
		err := c.DeleteKey(ctx, id)
		if err == nil {
			return nil
		}
		if !IsRetryable(err) {
			return err
		}
		lastErr = err
		if attempt == len(deleteKeyBackoff) {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(deleteKeyBackoff[attempt]):
		}
	}
	return lastErr
}

func (c *HTTPClient) GetBucketInfo(ctx context.Context, alias string) (*Bucket, error) {
	query := url.Values{}
	query.Set("globalAlias", alias)
	var out bucketInfo
	if err := c.do(ctx, http.MethodGet, "/v2/GetBucketInfo", query, nil, &out); err != nil {
		return nil, err
	}
	if out.ID == "" {
		return nil, fmt.Errorf("garage GetBucketInfo returned no id for %s", alias)
	}
	return &Bucket{ID: out.ID}, nil
}

func (c *HTTPClient) AllowBucketKey(ctx context.Context, bucketID, accessKeyID string, perms Permissions) error {
	req := allowBucketKeyRequest{
		BucketID:    bucketID,
		AccessKeyID: accessKeyID,
		Permissions: bucketKeyPerm(perms),
	}
	return c.do(ctx, http.MethodPost, "/v2/AllowBucketKey", nil, req, nil)
}

func (c *HTTPClient) do(ctx context.Context, method, path string, query url.Values, body any, out any) error {
	u, err := url.Parse(c.address + path)
	if err != nil {
		return err
	}
	if query != nil {
		u.RawQuery = query.Encode()
	}
	var rdr io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rdr = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, u.String(), rdr)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := strings.TrimSpace(string(payload))
		if msg == "" {
			msg = resp.Status
		}
		return &APIError{
			Method:     method,
			Path:       path,
			StatusCode: resp.StatusCode,
			Message:    msg,
		}
	}
	if out == nil || len(payload) == 0 {
		return nil
	}
	return json.Unmarshal(payload, out)
}
