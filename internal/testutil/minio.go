package testutil

import (
	"context"
	"fmt"
	"testing"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// StartMinIO starts an ephemeral MinIO server and returns its endpoint and
// credentials. Tests are skipped when Docker is unavailable. MinIO is the
// S3-compatible backend used by the storage module's integration tests.
func StartMinIO(t *testing.T) (endpoint, accessKey, secretKey string, useSSL bool) {
	t.Helper()

	ctx := context.Background()
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image: "minio/minio:RELEASE.2025-09-07T16-13-09Z",
			Env: map[string]string{
				"MINIO_ROOT_USER":     "minioadmin",
				"MINIO_ROOT_PASSWORD": "minioadmin",
			},
			ExposedPorts: []string{"9000/tcp"},
			Cmd:          []string{"server", "/data"},
			WaitingFor:   wait.ForLog("API: ").WithOccurrence(1),
		},
		Started: true,
	})
	if err != nil {
		t.Skipf("integration test requires Docker: %v", err)
	}

	t.Cleanup(func() {
		if err := container.Terminate(context.Background()); err != nil {
			t.Errorf("terminate MinIO container: %v", err)
		}
	})

	host, err := container.Host(ctx)
	if err != nil {
		t.Fatalf("get MinIO container host: %v", err)
	}
	port, err := container.MappedPort(ctx, "9000")
	if err != nil {
		t.Fatalf("get MinIO container port: %v", err)
	}

	return fmt.Sprintf("%s:%s", host, port.Port()), "minioadmin", "minioadmin", false
}
