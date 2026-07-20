// Package tools implements the Tool Registry module.
//
// Interface: ListTools(tenantID), Invoke(tenantID, name, params).
// Tool implementations (web search, CRM query) are internal seams
// hidden behind the registry.
package tools

import (
	"context"
	"fmt"
	"sync"

	"github.com/agentic-demo/platform/internal/domain"
	"github.com/agentic-demo/platform/internal/usage"
)

// ToolRegistry is the module interface.
type ToolRegistry interface {
	ListTools(ctx context.Context, tenantID domain.TenantID) []domain.ToolSchema
	Invoke(ctx context.Context, tenantID domain.TenantID, name string, params map[string]interface{}) domain.ToolResult
}

// Tool is an internal seam — a single tool implementation.
type Tool interface {
	Schema() domain.ToolSchema
	Execute(ctx context.Context, params map[string]interface{}) domain.ToolResult
}

// registry implements ToolRegistry.
type registry struct {
	mu          sync.RWMutex
	tools       map[string]Tool
	permissions map[string]map[string]bool // tenantID → toolName → allowed
	emitter     usage.UsageEmitter
}

func NewRegistry(emitter usage.UsageEmitter) ToolRegistry {
	return &registry{
		tools:       make(map[string]Tool),
		permissions: make(map[string]map[string]bool),
		emitter:     emitter,
	}
}

func (r *registry) ListTools(ctx context.Context, tenantID domain.TenantID) []domain.ToolSchema {
	r.mu.RLock()
	defer r.mu.RUnlock()

	allowed := r.permissions[string(tenantID)]

	var schemas []domain.ToolSchema
	for name, tool := range r.tools {
		// If permissions are configured, only return allowed tools.
		if allowed != nil && !allowed[name] {
			continue
		}
		schemas = append(schemas, tool.Schema())
	}
	return schemas
}

func (r *registry) Invoke(ctx context.Context, tenantID domain.TenantID, name string, params map[string]interface{}) domain.ToolResult {
	r.mu.RLock()
	tool, ok := r.tools[name]
	allowed := r.permissions[string(tenantID)]
	r.mu.RUnlock()

	if !ok {
		return domain.ToolResult{Error: fmt.Sprintf("unknown tool: %s", name)}
	}

	// Check permission.
	if allowed != nil && !allowed[name] {
		return domain.ToolResult{Error: fmt.Sprintf("tool %s not permitted for this tenant", name)}
	}

	return tool.Execute(ctx, params)
}
