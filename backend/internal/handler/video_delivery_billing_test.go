package handler

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

type videoDeliveryUsageRepo struct {
	service.UsageLogRepository
	log *service.UsageLog
}

func (r *videoDeliveryUsageRepo) Create(_ context.Context, log *service.UsageLog) (bool, error) {
	r.log = log
	return true, nil
}

type videoDeliveryBillingRepo struct {
	service.UsageBillingRepository
	command *service.UsageBillingCommand
}

func (r *videoDeliveryBillingRepo) Apply(_ context.Context, command *service.UsageBillingCommand) (*service.UsageBillingApplyResult, error) {
	r.command = command
	// Exercise the real durable billing command without mutating a balance.
	return &service.UsageBillingApplyResult{Applied: false}, nil
}

func newMappedVideoDeliveryService(t *testing.T, source, mappedModel string) (*service.OpenAIGatewayService, *videoDeliveryUsageRepo, *videoDeliveryBillingRepo) {
	t.Helper()
	requestPrice, mappedPrice, changedPrice := 0.2, 0.8, 9.0
	repo := &imageStudioPricingChannelRepo{channels: []service.Channel{{
		ID: 1, GroupIDs: []int64{7}, Status: service.StatusActive, BillingModelSource: source,
		ModelMapping: map[string]map[string]string{service.PlatformOpenAI: {"muse-video": mappedModel}},
		ModelPricing: []service.ChannelModelPricing{
			{Platform: service.PlatformOpenAI, Models: []string{"muse-video"}, BillingMode: service.BillingModePerRequest, PerRequestPrice: &requestPrice},
			{Platform: service.PlatformOpenAI, Models: []string{"muse-priced"}, BillingMode: service.BillingModePerRequest, PerRequestPrice: &mappedPrice},
			{Platform: service.PlatformOpenAI, Models: []string{"muse-changed"}, BillingMode: service.BillingModePerRequest, PerRequestPrice: &changedPrice},
		},
	}}}
	channel := service.NewChannelService(repo, nil, nil, nil, nil)
	cfg := &config.Config{}
	billing := service.NewBillingService(cfg, nil)
	resolver := service.NewModelPricingResolver(channel, billing)
	usage, charge := &videoDeliveryUsageRepo{}, &videoDeliveryBillingRepo{}
	svc := service.NewOpenAIGatewayService(
		nil, usage, charge, nil, nil, nil, nil, cfg, nil, nil, billing,
		nil, nil, nil, &service.DeferredService{}, nil, nil, resolver, channel, nil, nil, nil,
	)
	return svc, usage, charge
}

func TestDeliveredMuseVideoBillingMatchesQuoteAndSnapshot(t *testing.T) {
	for _, source := range []string{service.BillingModelSourceChannelMapped, service.BillingModelSourceRequested} {
		for _, snapshot := range []bool{true, false} {
			name := source + "/legacy"
			if snapshot {
				name = source + "/snapshot"
			}
			t.Run(name, func(t *testing.T) {
				svc, usage, charge := newMappedVideoDeliveryService(t, source, "muse-priced")
				groupID := int64(7)
				key := &service.APIKey{ID: 9, UserID: 8, User: &service.User{ID: 8}, GroupID: &groupID,
					Group: &service.Group{ID: groupID, Platform: service.PlatformOpenAI, RateMultiplier: 0.5}}
				quote, err := svc.VideoStudioPricing(context.Background(), key, "muse-video")
				require.NoError(t, err)
				require.Contains(t, quote.Tiers, "auto", "a pricing alias cannot change Muse's capabilities")
				wantBillingModel := "muse-priced"
				if source == service.BillingModelSourceRequested {
					wantBillingModel = "muse-video"
				}
				require.Equal(t, wantBillingModel, quote.BillingModel)
				record := &service.VideoTaskRecord{
					ID: "muse-delivered", UserID: 8, APIKeyID: 9, GroupID: groupID, AccountID: 3,
					CreatedAt: time.Now().Add(-time.Minute).Unix(), VideoURL: "https://storage.example/video.mp4", BrowserPlayable: true,
					UpstreamModel: "muse-video", Metadata: service.VideoTaskMetadata{Model: "muse-video", Resolution: "auto", Duration: 60},
				}
				if snapshot {
					record.BillingModel = quote.BillingModel
					// Channel rules can change while an asynchronous job is running;
					// the task must still use its accepted billing model.
					svc, usage, charge = newMappedVideoDeliveryService(t, service.BillingModelSourceChannelMapped, "muse-changed")
				}
				err = recordDeliveredGrokVideoUsage(&OpenAIGatewayHandler{gatewayService: svc}, key, nil,
					&service.Account{ID: 3, Platform: service.PlatformOpenAI}, record)
				require.NoError(t, err)
				require.NotNil(t, charge.command)
				require.InDelta(t, quote.Tiers["auto"].UnitPrice, charge.command.BalanceCost, 1e-12)
				require.Equal(t, "grok-video:muse-delivered", charge.command.RequestID)
				require.NotNil(t, usage.log)
				require.Equal(t, "muse-video", usage.log.RequestedModel)
				require.Equal(t, "auto", *usage.log.VideoResolution)
				require.Equal(t, 60, *usage.log.VideoDurationSeconds)
				require.Equal(t, "/v1/videos", *usage.log.UpstreamEndpoint)
				require.Equal(t, string(service.BillingModePerRequest), *usage.log.BillingMode)
			})
		}
	}
}
