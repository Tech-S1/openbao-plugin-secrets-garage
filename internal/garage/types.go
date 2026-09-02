package garage

import "time"

type Key struct {
	AccessKeyID     string
	SecretAccessKey string
	Expiration      *time.Time
}

type Bucket struct {
	ID string
}

type Permissions struct {
	Read  bool
	Write bool
	Owner bool
}

type createKeyRequest struct {
	Name         string    `json:"name"`
	Expiration   time.Time `json:"expiration"`
	NeverExpires bool      `json:"neverExpires"`
}

type updateKeyRequest struct {
	Name         string    `json:"name,omitempty"`
	Expiration   time.Time `json:"expiration"`
	NeverExpires bool      `json:"neverExpires"`
}

type keyInfo struct {
	AccessKeyID     string  `json:"accessKeyId"`
	SecretAccessKey *string `json:"secretAccessKey"`
	Expiration      *string `json:"expiration"`
}

type bucketInfo struct {
	ID string `json:"id"`
}

type bucketKeyPerm struct {
	Read  bool `json:"read"`
	Write bool `json:"write"`
	Owner bool `json:"owner"`
}

type allowBucketKeyRequest struct {
	BucketID    string        `json:"bucketId"`
	AccessKeyID string        `json:"accessKeyId"`
	Permissions bucketKeyPerm `json:"permissions"`
}
