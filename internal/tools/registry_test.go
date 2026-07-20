package tools

import (
	"context"
	"testing"

	"github.com/agentic-demo/platform/internal/domain"
	"github.com/agentic-demo/platform/internal/usage"
)

// mockTool implements Tool with a fixed schema and result.
type mockTool struct {
	name   string
	result domain.ToolResult
}

func (m *mockTool) Schema() domain.ToolSchema {
	return domain.ToolSchema{
		Name:        m.name,
		Description: "mock tool for testing",
		Parameters:  map[string]interface{}{"type": "object"},
	}
}

func (m *mockTool) Execute(ctx context.Context, params map[string]interface{}) domain.ToolResult {
	return m.result
}

func TestRegistry_InvokeRegisteredTool(t *testing.T) {
	reg := NewRegistry(usage.NoOpEmitter{}).(*registry)
	tool := &mockTool{
		name:   "test_tool",
		result: domain.ToolResult{Output: "tool executed"},
	}
	reg.Register("test_tool", tool)

	result := reg.Invoke(context.Background(), "tenant-a", "test_tool", nil)
	if result.Error != "" {
		t.Errorf("unexpected error: %s", result.Error)
	}
	if result.Output != "tool executed" {
		t.Errorf("Output = %q, want %q", result.Output, "tool executed")
	}
}

func TestRegistry_InvokeUnknownTool(t *testing.T) {
	reg := NewRegistry(usage.NoOpEmitter{}).(*registry)

	result := reg.Invoke(context.Background(), "tenant-a", "nonexistent", nil)
	if result.Error == "" {
		t.Fatal("expected error for unknown tool, got none")
	}
	if result.Error != "unknown tool: nonexistent" {
		t.Errorf("Error = %q, want %q", result.Error, "unknown tool: nonexistent")
	}
}

func TestRegistry_PermissionDenied(t *testing.T) {
	reg := NewRegistry(usage.NoOpEmitter{}).(*registry)
	tool := &mockTool{
		name:   "restricted_tool",
		result: domain.ToolResult{Output: "should not execute"},
	}
	reg.Register("restricted_tool", tool)
	reg.SetPermission("tenant-a", "restricted_tool", false)

	result := reg.Invoke(context.Background(), "tenant-a", "restricted_tool", nil)
	if result.Error == "" {
		t.Fatal("expected permission error, got none")
	}
	if result.Error != "tool restricted_tool not permitted for this tenant" {
		t.Errorf("Error = %q, want %q", result.Error, "tool restricted_tool not permitted for this tenant")
	}
}

func TestRegistry_ListToolsWithPermissions(t *testing.T) {
	reg := NewRegistry(usage.NoOpEmitter{}).(*registry)
	toolA := &mockTool{name: "tool_a"}
	toolB := &mockTool{name: "tool_b"}
	reg.Register("tool_a", toolA)
	reg.Register("tool_b", toolB)
	reg.SetPermission("tenant-a", "tool_a", true)
	reg.SetPermission("tenant-a", "tool_b", false)

	schemas := reg.ListTools(context.Background(), "tenant-a")
	if len(schemas) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(schemas))
	}
	if schemas[0].Name != "tool_a" {
		t.Errorf("Name = %q, want %q", schemas[0].Name, "tool_a")
	}
}

func TestRegistry_ListToolsNoPermissions(t *testing.T) {
	reg := NewRegistry(usage.NoOpEmitter{}).(*registry)
	toolA := &mockTool{name: "tool_a"}
	toolB := &mockTool{name: "tool_b"}
	reg.Register("tool_a", toolA)
	reg.Register("tool_b", toolB)

	// No permissions set → all tools should be returned.
	schemas := reg.ListTools(context.Background(), "tenant-a")
	if len(schemas) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(schemas))
	}
}