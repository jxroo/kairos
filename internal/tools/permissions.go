package tools

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/jxroo/kairos/internal/config"
	"go.uber.org/zap"
)

// ErrPermissionDenied is returned when a tool is denied access to a resource.
var ErrPermissionDenied = fmt.Errorf("permission denied")

// PermissionChecker evaluates tool permissions against configured rules.
// Rules are evaluated first-match-wins; no match defaults to deny.
type PermissionChecker struct {
	rules  []config.PermissionRule
	logger *zap.Logger
}

// NewPermissionChecker creates a PermissionChecker with the given rules.
func NewPermissionChecker(rules []config.PermissionRule, logger *zap.Logger) *PermissionChecker {
	return &PermissionChecker{rules: rules, logger: logger}
}

// Check evaluates whether toolName may access the given resource (and optional
// path). Returns nil if allowed, ErrPermissionDenied otherwise.
func (pc *PermissionChecker) Check(toolName, resource, path string) error {
	for _, rule := range pc.rules {
		if !matchTool(rule.Tool, toolName) {
			continue
		}
		if rule.Resource != resource {
			continue
		}
		if !rule.Allow {
			pc.logger.Debug("permission denied by rule",
				zap.String("tool", toolName),
				zap.String("resource", resource),
			)
			return ErrPermissionDenied
		}
		// If paths are specified, check that the requested path falls within.
		if len(rule.Paths) > 0 && path != "" {
			if !pathAllowed(rule.Paths, path) {
				pc.logger.Debug("permission denied: path not in allowed set",
					zap.String("tool", toolName),
					zap.String("path", path),
				)
				return ErrPermissionDenied
			}
		}
		return nil
	}
	// No matching rule → deny.
	return ErrPermissionDenied
}

func matchTool(pattern, toolName string) bool {
	return pattern == "*" || pattern == toolName
}

// evalSymlinksExisting resolves symlinks for the longest existing prefix of path.
// This handles paths where the leaf may not yet exist (e.g., a file about to be written).
func evalSymlinksExisting(path string) string {
	resolved, err := filepath.EvalSymlinks(path)
	if err == nil {
		return resolved
	}
	// Walk up to find existing parent, then append remaining segments.
	dir := filepath.Dir(path)
	base := filepath.Base(path)
	if dir == path {
		// Reached root without resolving — return as-is.
		return path
	}
	return filepath.Join(evalSymlinksExisting(dir), base)
}

func pathAllowed(allowed []string, target string) bool {
	abs, err := filepath.Abs(target)
	if err != nil {
		return false
	}
	resolved := evalSymlinksExisting(abs)
	for _, p := range allowed {
		ap, err := filepath.Abs(p)
		if err != nil {
			continue
		}
		resolvedAllowed := evalSymlinksExisting(ap)
		if strings.HasPrefix(resolved, resolvedAllowed) {
			return true
		}
	}
	return false
}
