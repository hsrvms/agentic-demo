package storage

import (
	"testing"

	"github.com/agentic-demo/platform/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestObjectKey_PrefixesTenant(t *testing.T) {
	key, err := objectKey(domain.TenantID("tenant-a"), "sources/s1/file")
	require.NoError(t, err)
	require.Equal(t, "tenant/tenant-a/sources/s1/file", key)
}

func TestObjectKey_RejectsInvalidKeys(t *testing.T) {
	tests := []struct {
		name string
		key  string
	}{
		{name: "empty", key: ""},
		{name: "absolute", key: "/etc/passwd"},
		{name: "tenant-prefixed", key: "tenant/tenant-b/secret"},
		{name: "traversal", key: "sources/../../tenant-b/secret"},
		{name: "traversal-middle", key: "sources/s1/../nested"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := objectKey(domain.TenantID("tenant-a"), tt.key)
			require.ErrorIs(t, err, ErrInvalidKey)
		})
	}
}

func TestObjectKey_AllowsNestedKeys(t *testing.T) {
	key, err := objectKey(domain.TenantID("tenant-a"), "sources/s1/attachments/notes.txt")
	require.NoError(t, err)
	require.Equal(t, "tenant/tenant-a/sources/s1/attachments/notes.txt", key)
}
