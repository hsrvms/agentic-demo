package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// ConnectionTester verifies that a data source is reachable and configured correctly.
type ConnectionTester interface {
	TestConnection(ctx context.Context, source *DataSource) (ConnectionTestResult, error)
}

// connectionTester implements ConnectionTester with per-type logic.
type connectionTester struct {
	httpClient *http.Client
}

// NewConnectionTester creates a ConnectionTester with a default HTTP client.
func NewConnectionTester() ConnectionTester {
	return &connectionTester{
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

func (t *connectionTester) TestConnection(ctx context.Context, source *DataSource) (ConnectionTestResult, error) {
	switch source.SourceType {
	case SourceTypeFileUpload:
		return t.testFileUpload(ctx, source)
	case SourceTypeWebsite:
		return t.testWebsite(ctx, source)
	case SourceTypeCRMHubSpot, SourceTypeCRMSalesforce:
		return t.testCRM(ctx, source)
	default:
		return ConnectionTestResult{
			Success: false,
			Message: fmt.Sprintf("unknown source type: %s", source.SourceType),
		}, nil
	}
}

func (t *connectionTester) testFileUpload(_ context.Context, _ *DataSource) (ConnectionTestResult, error) {
	return ConnectionTestResult{
		Success: true,
		Message: "file_upload sources do not require connection testing",
	}, nil
}

func (t *connectionTester) testWebsite(ctx context.Context, source *DataSource) (ConnectionTestResult, error) {
	var cfg struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(source.Config, &cfg); err != nil || cfg.URL == "" {
		return ConnectionTestResult{
			Success: false,
			Message: "website config must include a valid url",
		}, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodHead, cfg.URL, http.NoBody)
	if err != nil {
		return ConnectionTestResult{
			Success: false,
			Message: fmt.Sprintf("invalid URL: %v", err),
		}, nil
	}

	start := time.Now()
	resp, err := t.httpClient.Do(req)
	latency := time.Since(start)
	if err != nil {
		return ConnectionTestResult{
			Success: false,
			Message: fmt.Sprintf("connection failed: %v", err),
			Latency: latency.String(),
		}, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return ConnectionTestResult{
			Success: false,
			Message: fmt.Sprintf("server returned status %d", resp.StatusCode),
			Latency: latency.String(),
		}, nil
	}

	return ConnectionTestResult{
		Success: true,
		Message: fmt.Sprintf("connection successful (HTTP %d)", resp.StatusCode),
		Latency: latency.String(),
	}, nil
}

func (t *connectionTester) testCRM(_ context.Context, source *DataSource) (ConnectionTestResult, error) {
	if len(source.Credentials) == 0 {
		return ConnectionTestResult{
			Success: false,
			Message: "CRM sources require credentials",
		}, nil
	}

	// Full CRM connection test would validate credentials against the API.
	// For now, presence of credentials is sufficient.
	return ConnectionTestResult{
		Success: true,
		Message: fmt.Sprintf("credentials configured for %s", source.SourceType),
	}, nil
}
