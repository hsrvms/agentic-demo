package sources

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConnectionTester_FileUpload(t *testing.T) {
	tester := NewConnectionTester()
	ctx := context.Background()

	source := DataSource{
		SourceType: SourceTypeFileUpload,
		Name:       "Upload Source",
		Config:     json.RawMessage(`{}`),
	}

	result, err := tester.TestConnection(ctx, &source)
	require.NoError(t, err)
	assert.True(t, result.Success)
	assert.Contains(t, result.Message, "do not require connection testing")
}

func TestConnectionTester_Website_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	tester := NewConnectionTester()
	ctx := context.Background()

	cfg, err := json.Marshal(map[string]string{"url": server.URL})
	require.NoError(t, err)

	source := DataSource{
		SourceType: SourceTypeWebsite,
		Name:       "Test Website",
		Config:     cfg,
	}

	result, err := tester.TestConnection(ctx, &source)
	require.NoError(t, err)
	assert.True(t, result.Success)
	assert.Contains(t, result.Message, "connection successful")
	assert.NotEmpty(t, result.Latency)
}

func TestConnectionTester_Website_ErrorStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	tester := NewConnectionTester()
	ctx := context.Background()

	cfg, err := json.Marshal(map[string]string{"url": server.URL})
	require.NoError(t, err)

	source := DataSource{
		SourceType: SourceTypeWebsite,
		Name:       "Test Website",
		Config:     cfg,
	}

	result, err := tester.TestConnection(ctx, &source)
	require.NoError(t, err)
	assert.False(t, result.Success)
	assert.Contains(t, result.Message, "404")
}

func TestConnectionTester_Website_InvalidURL(t *testing.T) {
	tester := NewConnectionTester()
	ctx := context.Background()

	source := DataSource{
		SourceType: SourceTypeWebsite,
		Name:       "Bad Config",
		Config:     json.RawMessage(`{"url": ""}`),
	}

	result, err := tester.TestConnection(ctx, &source)
	require.NoError(t, err)
	assert.False(t, result.Success)
	assert.Contains(t, result.Message, "url")
}

func TestConnectionTester_Website_Unreachable(t *testing.T) {
	tester := NewConnectionTester()
	ctx := context.Background()

	cfg, err := json.Marshal(map[string]string{"url": "http://127.0.0.1:1"})
	require.NoError(t, err)

	source := DataSource{
		SourceType: SourceTypeWebsite,
		Name:       "Unreachable",
		Config:     cfg,
	}

	result, err := tester.TestConnection(ctx, &source)
	require.NoError(t, err)
	assert.False(t, result.Success)
	assert.Contains(t, result.Message, "connection failed")
}

func TestConnectionTester_CRM_WithCredentials(t *testing.T) {
	tester := NewConnectionTester()
	ctx := context.Background()

	source := DataSource{
		SourceType:  SourceTypeCRMHubSpot,
		Name:        "HubSpot",
		Config:      json.RawMessage(`{}`),
		Credentials: []byte("encrypted-creds"),
	}

	result, err := tester.TestConnection(ctx, &source)
	require.NoError(t, err)
	assert.True(t, result.Success)
	assert.Contains(t, result.Message, "credentials configured")
}

func TestConnectionTester_CRM_NoCredentials(t *testing.T) {
	tester := NewConnectionTester()
	ctx := context.Background()

	source := DataSource{
		SourceType: SourceTypeCRMSalesforce,
		Name:       "Salesforce",
		Config:     json.RawMessage(`{}`),
	}

	result, err := tester.TestConnection(ctx, &source)
	require.NoError(t, err)
	assert.False(t, result.Success)
	assert.Contains(t, result.Message, "require credentials")
}

func TestConnectionTester_UnknownType(t *testing.T) {
	tester := NewConnectionTester()
	ctx := context.Background()

	source := DataSource{
		SourceType: "unknown",
		Name:       "Unknown",
		Config:     json.RawMessage(`{}`),
	}

	result, err := tester.TestConnection(ctx, &source)
	require.NoError(t, err)
	assert.False(t, result.Success)
	assert.Contains(t, result.Message, "unknown source type")
}
