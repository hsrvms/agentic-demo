package sources

import "errors"

var (
	ErrNotFound         = errors.New("data source not found")
	ErrInvalidTenantID  = errors.New("tenant_id must not be empty")
	ErrInvalidSourceID  = errors.New("source ID is invalid")
	ErrInvalidName      = errors.New("name must not be empty")
	ErrInvalidSourceType = errors.New("invalid source type")
	ErrInvalidConfig    = errors.New("config must be valid JSON")
	ErrEncryptionFailed = errors.New("failed to encrypt credentials")
	ErrDecryptionFailed = errors.New("failed to decrypt credentials")
	ErrConnectionFailed = errors.New("connection test failed")
)