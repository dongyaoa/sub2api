package service

import (
	"context"
	"sort"
	"strings"
)

// ImageStudioModel is a model whose image capability is known from account
// mappings, its public name, or a model-level image pricing card.
type ImageStudioModel struct {
	ID              string `json:"id"`
	Object          string `json:"object"`
	DisplayName     string `json:"display_name"`
	ImageGeneration bool   `json:"image_generation"`
}

// ImageStudioModels deliberately has its own catalog rules: an explicit group
// allowlist may introduce an upstream alias that is newer than our defaults.
// Ordinary /v1/models clients retain their existing intersection behavior.
func (s *GatewayService) ImageStudioModels(ctx context.Context, group *Group, platform string, defaults []string) ([]ImageStudioModel, error) {
	models := make([]ImageStudioModel, 0)
	if s == nil || s.accountRepo == nil || group == nil {
		return models, nil
	}
	accounts, err := s.accountRepo.ListSchedulableByGroupID(ctx, group.ID)
	if err != nil {
		return nil, err
	}

	candidates := make(map[string]struct{})
	eligibleAccounts := make([]*Account, 0, len(accounts))
	add := func(model string) {
		model = imageStudioModelID(model)
		if model != "" && !strings.Contains(model, "*") {
			candidates[model] = struct{}{}
		}
	}
	hasUnmappedAccount := false
	for i := range accounts {
		account := &accounts[i]
		if account.Platform != platform && !mixedListingAccountAllowed(platform, account) {
			continue
		}
		eligibleAccounts = append(eligibleAccounts, account)
		mapping := account.GetModelMapping()
		if len(mapping) == 0 || account.IsOpenAIPassthroughEnabled() {
			hasUnmappedAccount = true
		}
		for alias := range mapping {
			if account.Platform != platform && !mixedListingModelAllowed(platform, alias) {
				continue
			}
			add(alias)
		}
		// Account model sync saves the upstream catalog here. Reading the saved
		// catalog avoids making the workbench depend on a live upstream GET.
		if snapshot := account.GetUpstreamModelMetadataSnapshot(); snapshot != nil {
			for id := range snapshot.Models {
				id = imageStudioModelID(id)
				if account.IsModelSupported(id) && (account.Platform == platform || mixedListingModelAllowed(platform, id)) {
					add(id)
				}
			}
		}
	}
	if len(candidates) == 0 || hasUnmappedAccount {
		for _, model := range defaults {
			add(model)
		}
	}
	if group.ModelAllowlistEnabled() {
		for _, model := range group.ModelAllowlist.Models {
			// Wildcards describe permission, not an upstream model ID.
			add(model)
		}
	}

	source := make([]string, 0, len(candidates))
	for model := range candidates {
		source = append(source, model)
	}
	sort.Strings(source)
	if group.ModelAllowlistEnabled() {
		allowlist := group.ModelAllowlist
		allowlist.Models = make([]string, len(group.ModelAllowlist.Models))
		for i, model := range group.ModelAllowlist.Models {
			allowlist.Models[i] = imageStudioModelID(model)
		}
		source = allowlist.FilterForListing(source)
	}
	for _, id := range source {
		channelTarget, allowed := s.imageStudioChannelModel(ctx, group.ID, id)
		if !allowed {
			continue
		}
		isImage := imageStudioModelName(id) || s.imageStudioImagePricing(ctx, group, id) ||
			imageStudioModelName(channelTarget) || s.imageStudioImagePricing(ctx, group, channelTarget)
		if !isImage {
			for _, account := range eligibleAccounts {
				if target, matched := account.ResolveMappedModel(channelTarget); matched &&
					(imageStudioModelName(target) || s.imageStudioImagePricing(ctx, group, target)) {
					isImage = true
					break
				}
			}
		}
		if isImage {
			models = append(models, ImageStudioModel{ID: id, Object: "model", DisplayName: id, ImageGeneration: true})
		}
	}
	return models, nil
}

func (s *GatewayService) imageStudioChannelModel(ctx context.Context, groupID int64, id string) (string, bool) {
	channelService := s.channelService
	if channelService == nil && s.resolver != nil {
		channelService = s.resolver.channelService
	}
	if channelService == nil {
		return id, true
	}
	mapping := channelService.ResolveChannelMapping(ctx, groupID, id)
	billingModel := billingModelForRestriction(mapping.BillingModelSource, id, mapping.MappedModel)
	if billingModel != "" && channelService.IsModelRestricted(ctx, groupID, billingModel) {
		return "", false
	}
	return mapping.MappedModel, true
}

func (s *GatewayService) imageStudioImagePricing(ctx context.Context, group *Group, id string) bool {
	if s.resolver == nil {
		return false
	}
	pricing := s.resolver.ResolveImagePricing(ctx, PricingInput{Model: id, GroupID: &group.ID, Group: group})
	return pricing != nil && (pricing.Mode == BillingModeImage || pricing.Mode == BillingModePerRequest)
}

func imageStudioModelID(model string) string {
	return strings.TrimPrefix(strings.TrimSpace(model), "models/")
}

func imageStudioModelName(model string) bool {
	model = strings.ToLower(imageStudioModelID(model))
	return strings.Contains(model, "image") || strings.Contains(model, "nano-banana") ||
		strings.Contains(model, "nanobanana") || strings.Contains(model, "nano_banana")
}
