package repository

import (
	"context"
	"fmt"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/apikey"
	"github.com/Wei-Shaw/sub2api/ent/group"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type channelMonitorAPIKeyGroupResolver struct {
	client *dbent.Client
}

// NewChannelMonitorAPIKeyGroupResolver resolves the current group for the
// locally-issued API keys used by V1 monitors.
func NewChannelMonitorAPIKeyGroupResolver(client *dbent.Client) service.ChannelMonitorAPIKeyGroupResolver {
	return &channelMonitorAPIKeyGroupResolver{client: client}
}

func (r *channelMonitorAPIKeyGroupResolver) ResolveByKeys(
	ctx context.Context,
	keys []string,
) (map[string]service.ChannelMonitorAPIKeyGroup, error) {
	out := make(map[string]service.ChannelMonitorAPIKeyGroup, len(keys))
	if len(keys) == 0 {
		return out, nil
	}
	rows, err := clientFromContext(ctx, r.client).APIKey.Query().
		Where(apikey.KeyIn(keys...)).
		Select(apikey.FieldKey, apikey.FieldGroupID).
		WithGroup(func(q *dbent.GroupQuery) {
			q.Select(group.FieldName, group.FieldRateMultiplier)
		}).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("query monitor api key groups: %w", err)
	}
	for _, row := range rows {
		resolvedGroup, err := row.Edges.GroupOrErr()
		if err != nil {
			continue
		}
		out[row.Key] = service.ChannelMonitorAPIKeyGroup{
			Name:           resolvedGroup.Name,
			RateMultiplier: resolvedGroup.RateMultiplier,
		}
	}
	return out, nil
}
