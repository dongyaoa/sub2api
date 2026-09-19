package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type monitorDisplayOrderStoreStub struct {
	value             string
	readErr, writeErr error
}

func (s *monitorDisplayOrderStoreStub) GetValue(_ context.Context, key string) (string, error) {
	if key != SettingKeyChannelMonitorDisplayOrder {
		panic("unexpected setting key")
	}
	return s.value, s.readErr
}

func (s *monitorDisplayOrderStoreStub) Set(_ context.Context, key, value string) error {
	if key != SettingKeyChannelMonitorDisplayOrder {
		panic("unexpected setting key")
	}
	if s.writeErr != nil {
		return s.writeErr
	}
	s.value = value
	return nil
}

type monitorDisplayOrderRepoStub struct {
	ChannelMonitorRepository
	monitors      []*ChannelMonitor
	latest        map[int64][]*ChannelMonitorLatest
	existing      []int64
	timelineLimit int
}

func (r *monitorDisplayOrderRepoStub) ExistingIDs(context.Context, []int64) ([]int64, error) {
	return r.existing, nil
}
func (r *monitorDisplayOrderRepoStub) ListEnabled(context.Context) ([]*ChannelMonitor, error) {
	return r.monitors, nil
}
func (r *monitorDisplayOrderRepoStub) ListLatestForMonitorIDs(context.Context, []int64) (map[int64][]*ChannelMonitorLatest, error) {
	return r.latest, nil
}
func (r *monitorDisplayOrderRepoStub) ComputeAvailabilityForMonitors(context.Context, []int64, int) (map[int64][]*ChannelMonitorAvailability, error) {
	return nil, nil
}
func (r *monitorDisplayOrderRepoStub) ListRecentHistoryForMonitors(_ context.Context, _ []int64, _ map[int64]string, limit int) (map[int64][]*ChannelMonitorHistoryEntry, error) {
	r.timelineLimit = limit
	return nil, nil
}

func TestChannelMonitorDisplayOrderPersistsAcrossServices(t *testing.T) {
	ctx := context.Background()
	store := &monitorDisplayOrderStoreStub{readErr: ErrSettingNotFound}
	svc := NewChannelMonitorService(&monitorDisplayOrderRepoStub{existing: []int64{8, 2}}, nil)
	svc.SetDisplayOrderStore(store)
	order, err := svc.GetDisplayOrder(ctx)
	require.NoError(t, err)
	require.Equal(t, DefaultChannelMonitorDisplayOrder(), order)

	saved := ChannelMonitorDisplayOrder{GroupOrder: []string{"other", "anthropic", "openai"}, MonitorOrder: []int64{8, 2}}
	require.NoError(t, svc.UpdateDisplayOrder(ctx, saved))
	store.readErr = nil
	other := NewChannelMonitorService(nil, nil)
	other.SetDisplayOrderStore(store)
	order, err = other.GetDisplayOrder(ctx)
	require.NoError(t, err)
	require.Equal(t, saved, order)
}

func TestChannelMonitorDisplayOrderRejectsInvalidAndStaleOrders(t *testing.T) {
	valid := []string{"openai", "anthropic", "other"}
	cases := []struct {
		name  string
		order ChannelMonitorDisplayOrder
	}{
		{"missing group", ChannelMonitorDisplayOrder{GroupOrder: valid[:2], MonitorOrder: []int64{}}},
		{"duplicate group", ChannelMonitorDisplayOrder{GroupOrder: []string{"openai", "openai", "other"}, MonitorOrder: []int64{}}},
		{"unknown group", ChannelMonitorDisplayOrder{GroupOrder: []string{"openai", "anthropic", "gemini"}, MonitorOrder: []int64{}}},
		{"duplicate ID", ChannelMonitorDisplayOrder{GroupOrder: valid, MonitorOrder: []int64{1, 1}}},
		{"zero ID", ChannelMonitorDisplayOrder{GroupOrder: valid, MonitorOrder: []int64{0}}},
		{"negative ID", ChannelMonitorDisplayOrder{GroupOrder: valid, MonitorOrder: []int64{-1}}},
		{"null IDs", ChannelMonitorDisplayOrder{GroupOrder: valid}},
		{"too many IDs", ChannelMonitorDisplayOrder{GroupOrder: valid, MonitorOrder: make([]int64, maxChannelMonitorDisplayOrderIDs+1)}},
		{"deleted monitor", ChannelMonitorDisplayOrder{GroupOrder: valid, MonitorOrder: []int64{999}}},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			store := &monitorDisplayOrderStoreStub{value: "unchanged"}
			svc := NewChannelMonitorService(&monitorDisplayOrderRepoStub{}, nil)
			svc.SetDisplayOrderStore(store)
			require.Error(t, svc.UpdateDisplayOrder(context.Background(), tt.order))
			require.Equal(t, "unchanged", store.value)
		})
	}
}

func TestChannelMonitorDisplayOrderSurfacesStorageFailures(t *testing.T) {
	store := &monitorDisplayOrderStoreStub{writeErr: errors.New("database unavailable"), readErr: errors.New("read unavailable")}
	svc := NewChannelMonitorService(nil, nil)
	svc.SetDisplayOrderStore(store)
	require.ErrorContains(t, svc.UpdateDisplayOrder(context.Background(), DefaultChannelMonitorDisplayOrder()), "save channel monitor display order")
	_, err := svc.GetDisplayOrder(context.Background())
	require.ErrorContains(t, err, "read channel monitor display order")
}

func TestChannelMonitorDisplayOrderUserViewSortsAndFiltersHiddenIDs(t *testing.T) {
	checkedAt := time.Date(2026, 9, 19, 9, 30, 0, 0, time.UTC)
	repo := &monitorDisplayOrderRepoStub{
		monitors: []*ChannelMonitor{
			{ID: 1, Provider: "openai", PrimaryModel: "gpt", IntervalSeconds: 60, JitterSeconds: 5},
			{ID: 4, Provider: "gemini"},
			{ID: 6, Provider: "openai"},
			{ID: 3, Provider: "anthropic"},
			{ID: 5, Provider: "grok"},
			{ID: 2, Provider: "openai"},
		},
		latest: map[int64][]*ChannelMonitorLatest{1: {{Model: "gpt", CheckedAt: checkedAt}}},
	}
	svc := NewChannelMonitorService(repo, nil)
	svc.SetDisplayOrderStore(&monitorDisplayOrderStoreStub{value: `{"group_order":["other","anthropic","openai"],"monitor_order":[99,5,2,1]}`})
	views, order, err := svc.ListUserViewWithDisplayOrder(context.Background())
	require.NoError(t, err)
	require.Equal(t, []int64{5, 4, 3, 2, 1, 6}, order.MonitorOrder)
	require.Equal(t, []string{"other", "anthropic", "openai"}, order.GroupOrder)
	for i, view := range views {
		require.Equal(t, order.MonitorOrder[i], view.ID)
	}
	require.Equal(t, 60, views[4].IntervalSeconds)
	require.Equal(t, 5, views[4].JitterSeconds)
	require.Equal(t, &checkedAt, views[4].LastCheckedAt)
	require.Nil(t, views[0].LastCheckedAt)
	require.Equal(t, 45, repo.timelineLimit)

	// Deleted/disabled IDs cannot escape in the metadata, including empty lists.
	repo.monitors = nil
	views, order, err = svc.ListUserViewWithDisplayOrder(context.Background())
	require.NoError(t, err)
	require.Empty(t, views)
	require.Equal(t, []int64{}, order.MonitorOrder)
}

func TestChannelMonitorTimelineCapsAt45RealPoints(t *testing.T) {
	entries := make([]*ChannelMonitorHistoryEntry, 60)
	for i := range entries {
		entries[i] = &ChannelMonitorHistoryEntry{Status: "operational", CheckedAt: time.Unix(int64(100-i), 0)}
	}
	points := buildTimelinePoints(entries)
	require.Len(t, points, 45)
	require.Equal(t, entries[0].CheckedAt, points[0].CheckedAt)
	require.Equal(t, entries[44].CheckedAt, points[44].CheckedAt)
}
