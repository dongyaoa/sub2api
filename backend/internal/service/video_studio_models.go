package service

import (
	"context"
	"sort"
	"strings"
)

// VideoStudioModels reads the configured upstream catalog without depending on
// a group's Codex-only model manifest or making live requests to providers.
func (s *GatewayService) VideoStudioModels(ctx context.Context, group *Group) ([]string, error) {
	models := make([]string, 0)
	if s == nil || s.accountRepo == nil || group == nil {
		return models, nil
	}
	accounts, err := s.accountRepo.ListSchedulableByGroupID(ctx, group.ID)
	if err != nil {
		return nil, err
	}
	candidates := make(map[string]struct{})
	for i := range accounts {
		account := &accounts[i]
		if group.Platform != PlatformComposite && account.Platform != group.Platform {
			continue
		}
		add := func(model string) {
			model = strings.TrimSpace(model)
			if group.ModelAllowlistEnabled() && len(group.ModelAllowlist.FilterForListing([]string{model})) == 0 {
				return
			}
			target, allowed := s.imageStudioChannelModel(ctx, group.ID, model)
			if !allowed || !account.IsModelSupported(target) {
				return
			}
			if IsMuseVideoModel(model) && SupportsMuseVideoAccount(account) {
				candidates[model] = struct{}{}
			}
			if IsGeminiOmniVideoModel(model) && SupportsGeminiOmniVideoAccount(account) {
				candidates[model] = struct{}{}
			}
			if account.Platform == PlatformGrok && isVideoStudioGrokModel(model) {
				candidates[model] = struct{}{}
			}
		}
		mapping := account.GetModelMapping()
		snapshot := account.GetUpstreamModelMetadataSnapshot()
		// A synchronized catalog is evidence of what a custom upstream offers.
		// Do not fill a text-only catalog with Grok's default media models. An
		// explicit administrator mapping still takes precedence over discovery.
		explicitMapping := stringMappingFromRaw(account.Credentials["model_mapping"])
		if len(explicitMapping) > 0 || snapshot == nil {
			for model := range mapping {
				add(model)
				if account.Platform == PlatformGrok && strings.HasSuffix(model, "*") {
					for _, candidate := range []string{"grok-imagine-video", "grok-imagine-video-1.5"} {
						if matchWildcard(model, candidate) {
							add(candidate)
						}
					}
				}
			}
		}
		if snapshot != nil {
			for model := range snapshot.Models {
				add(model)
			}
		}
		if group.ModelAllowlistEnabled() {
			for _, model := range group.ModelAllowlist.Models {
				add(model)
			}
		}
	}
	for model := range candidates {
		models = append(models, model)
	}
	sort.Strings(models)
	return models, nil
}

func isVideoStudioGrokModel(model string) bool {
	switch strings.ToLower(strings.TrimSpace(model)) {
	case "grok-imagine-video", "grok-imagine-video-1.5", "svggrok-imagine-video-1.5":
		return true
	default:
		return false
	}
}
