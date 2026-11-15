package backend

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/openbao/openbao/sdk/v2/logical"
)

const (
	storageConfigKey   = "config"
	storageRolesPrefix = "roles/"

	defaultTTL    = 5 * time.Minute
	defaultMaxTTL = 15 * time.Minute
)

type Config struct {
	Address    string        `json:"address"`
	Token      string        `json:"token"`
	DefaultTTL time.Duration `json:"default_ttl"`
	MaxTTL     time.Duration `json:"max_ttl"`
}

type Role struct {
	Bucket string        `json:"bucket"`
	TTL    time.Duration `json:"ttl"`
	MaxTTL time.Duration `json:"max_ttl"`
	Read   bool          `json:"read"`
	Write  bool          `json:"write"`
	Owner  bool          `json:"owner"`
}

func (b *Backend) getConfig(ctx context.Context, s logical.Storage) (*Config, error) {
	raw, err := s.Get(ctx, storageConfigKey)
	if err != nil {
		return nil, err
	}
	if raw == nil {
		return nil, nil
	}
	var cfg Config
	if err := json.Unmarshal(raw.Value, &cfg); err != nil {
		return nil, fmt.Errorf("failed to decode config: %w", err)
	}
	return &cfg, nil
}

func (b *Backend) getRole(ctx context.Context, s logical.Storage, name string) (*Role, error) {
	raw, err := s.Get(ctx, storageRolesPrefix+name)
	if err != nil {
		return nil, err
	}
	if raw == nil {
		return nil, nil
	}
	var role Role
	if err := json.Unmarshal(raw.Value, &role); err != nil {
		return nil, fmt.Errorf("failed to decode role %s: %w", name, err)
	}
	return &role, nil
}

func resolveTTL(roleTTL, roleMaxTTL, cfgDefaultTTL, cfgMaxTTL time.Duration) (ttl, maxTTL time.Duration) {
	ttl = roleTTL
	if ttl == 0 {
		ttl = cfgDefaultTTL
	}
	if ttl == 0 {
		ttl = defaultTTL
	}
	maxTTL = roleMaxTTL
	if maxTTL == 0 {
		maxTTL = cfgMaxTTL
	}
	if maxTTL > 0 && ttl > maxTTL {
		ttl = maxTTL
	}
	return ttl, maxTTL
}

func (b *Backend) clampLeaseTTLs(ttl, maxTTL time.Duration) (time.Duration, time.Duration) {
	if sys := b.System(); sys != nil {
		if sysMax := sys.MaxLeaseTTL(); sysMax > 0 {
			if maxTTL == 0 || maxTTL > sysMax {
				maxTTL = sysMax
			}
			if ttl > sysMax {
				ttl = sysMax
			}
		}
	}
	if maxTTL > 0 && ttl > maxTTL {
		ttl = maxTTL
	}
	return ttl, maxTTL
}

func renewLeaseTTL(secret *logical.Secret, snapshotTTL, snapshotMaxTTL time.Duration, now time.Time) (ttl, maxTTL time.Duration) {
	maxTTL = snapshotMaxTTL
	ttl = snapshotTTL
	if secret != nil && secret.Increment > 0 {
		ttl = secret.Increment
	}
	if maxTTL > 0 && secret != nil && !secret.IssueTime.IsZero() {
		remaining := maxTTL - now.Sub(secret.IssueTime)
		if remaining < 0 {
			remaining = 0
		}
		if ttl > remaining {
			ttl = remaining
		}
	}
	if maxTTL > 0 && ttl > maxTTL {
		ttl = maxTTL
	}
	return ttl, maxTTL
}
