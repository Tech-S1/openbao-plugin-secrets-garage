package backend

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/ZachTech-HomeLab/vault-plugin-secrets-garage/internal/garage"
)

func (b *Backend) provisionKey(ctx context.Context, client garage.Client, role *Role, roleName string, expiration time.Time) (*garage.Key, error) {
	keyName := "vault-garage-" + roleName + "-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	key, err := client.CreateKey(ctx, keyName, expiration)
	if err != nil {
		return nil, fmt.Errorf("CreateKey: %w", err)
	}
	if role.Bucket == "" {
		return key, nil
	}
	bucket, err := client.GetBucketInfo(ctx, role.Bucket)
	if err != nil {
		_ = client.DeleteKey(ctx, key.AccessKeyID)
		return nil, fmt.Errorf("GetBucketInfo: %w", err)
	}
	perms := garage.Permissions{Read: role.Read, Write: role.Write, Owner: role.Owner}
	if err := client.AllowBucketKey(ctx, bucket.ID, key.AccessKeyID, perms); err != nil {
		_ = client.DeleteKey(ctx, key.AccessKeyID)
		return nil, fmt.Errorf("AllowBucketKey: %w", err)
	}
	return key, nil
}
