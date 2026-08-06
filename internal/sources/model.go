// Package sources implements the Data Source Registry domain.
// Tenants configure data sources (file uploads, websites, CRMs) for ingestion.
// Credentials are encrypted at rest using AES-256-GCM.
package sources

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// SourceType enumerates the supported data source types.
type SourceType string

const (
	SourceTypeFileUpload    SourceType = "file_upload"
	SourceTypeWebsite       SourceType = "website"
	SourceTypeCRMHubSpot    SourceType = "crm_hubspot"
	SourceTypeCRMSalesforce SourceType = "crm_salesforce"
)

// ValidSourceType returns true if t is a known source type.
func ValidSourceType(t string) bool {
	switch SourceType(t) {
	case SourceTypeFileUpload, SourceTypeWebsite, SourceTypeCRMHubSpot, SourceTypeCRMSalesforce:
		return true
	}
	return false
}

// Status represents the lifecycle state of a data source.
type Status string

const (
	StatusInactive Status = "inactive"
	StatusActive   Status = "active"
	StatusError    Status = "error"
)

// DataSource is the domain model for a configured data source.
type DataSource struct {
	ID             uuid.UUID
	TenantID       string
	SourceType     SourceType
	Name           string
	Config         json.RawMessage
	Credentials    []byte // encrypted at rest; nil when no credentials
	Status         Status
	LastSyncAt     *time.Time
	LastSyncStatus string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// CreateDataSourceParams holds the inputs for creating a new data source.
type CreateDataSourceParams struct {
	TenantID    string
	SourceType  SourceType
	Name        string
	Config      json.RawMessage
	Credentials []byte // plaintext; will be encrypted by the service
	File        []byte // file_upload only: raw bytes persisted to the object store
}

// UpdateDataSourceParams holds the inputs for updating an existing data source.
// nil fields are left unchanged.
type UpdateDataSourceParams struct {
	Name        *string
	Config      *json.RawMessage
	Credentials *[]byte // plaintext; will be encrypted by the service
	Status      *Status
	File        []byte // file_upload only: replaces the stored object
}

// DataSourcePage is a paginated list of data sources.
type DataSourcePage struct {
	Sources    []DataSource
	TotalCount int
	Page       int
	PageSize   int
}

// ConnectionTestResult describes the outcome of a connection test.
type ConnectionTestResult struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Latency string `json:"latency,omitempty"`
}
