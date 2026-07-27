package agent

import (
	"context"
	"strings"

	"github.com/MISSmihu/MHcode/internal/tools"
)

type turnWritableRootsKey struct{}

func withTurnWritableRoots(ctx context.Context, roots []string) context.Context {
	if len(roots) == 0 {
		return ctx
	}
	return context.WithValue(ctx, turnWritableRootsKey{}, append([]string(nil), roots...))
}

func turnWritableRoots(ctx context.Context) []string {
	if ctx == nil {
		return nil
	}
	roots, _ := ctx.Value(turnWritableRootsKey{}).([]string)
	return append([]string(nil), roots...)
}

func (s *Service) prepareTurnPathAccess(ctx context.Context, prompt string) (context.Context, []string, error) {
	roots := explicitTurnPathGrants(prompt)
	if !strings.EqualFold(strings.TrimSpace(s.runtimeSettings.FilesystemAccess), "unrestricted") {
		for _, root := range roots {
			if err := validateExplicitTurnPathGrant(root); err != nil {
				return ctx, nil, err
			}
		}
	}
	return withTurnWritableRoots(ctx, roots), roots, nil
}

func (s *Service) sandboxPolicyForContext(ctx context.Context) tools.SandboxPolicy {
	policy := s.sandboxPolicy()
	policy.ExtraWritableRoots = mergeTurnRoots(policy.ExtraWritableRoots, turnWritableRoots(ctx))
	return policy
}

func mergeTurnRoots(configured, temporary []string) []string {
	merged := make([]string, 0, len(configured)+len(temporary))
	for _, candidate := range append(append([]string(nil), configured...), temporary...) {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		duplicate := false
		for _, existing := range merged {
			if strings.EqualFold(existing, candidate) {
				duplicate = true
				break
			}
		}
		if !duplicate {
			merged = append(merged, candidate)
		}
	}
	return merged
}
