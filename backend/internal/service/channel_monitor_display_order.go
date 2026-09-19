package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const SettingKeyChannelMonitorDisplayOrder = "channel_monitor_display_order"
const maxChannelMonitorDisplayOrderIDs = 10000

// ChannelMonitorDisplayOrder is shared by every viewer. IDs omitted from the
// saved order are appended by ID within their platform group.
type ChannelMonitorDisplayOrder struct {
	GroupOrder   []string `json:"group_order"`
	MonitorOrder []int64  `json:"monitor_order"`
}

type channelMonitorDisplayOrderStore interface {
	GetValue(context.Context, string) (string, error)
	Set(context.Context, string, string) error
}

func (s *ChannelMonitorService) SetDisplayOrderStore(store channelMonitorDisplayOrderStore) {
	s.displayOrderStore = store
}

func DefaultChannelMonitorDisplayOrder() ChannelMonitorDisplayOrder {
	return ChannelMonitorDisplayOrder{
		GroupOrder:   []string{MonitorProviderOpenAI, MonitorProviderAnthropic, "other"},
		MonitorOrder: []int64{},
	}
}

func validateChannelMonitorDisplayOrder(order ChannelMonitorDisplayOrder) error {
	invalid := func(message string) error {
		return infraerrors.BadRequest("INVALID_CHANNEL_MONITOR_DISPLAY_ORDER", message)
	}
	if len(order.GroupOrder) != 3 {
		return invalid("group_order must contain openai, anthropic and other exactly once")
	}
	groups := make(map[string]bool, 3)
	for _, group := range order.GroupOrder {
		if (group != MonitorProviderOpenAI && group != MonitorProviderAnthropic && group != "other") || groups[group] {
			return invalid("group_order must contain openai, anthropic and other exactly once")
		}
		groups[group] = true
	}
	if order.MonitorOrder == nil || len(order.MonitorOrder) > maxChannelMonitorDisplayOrderIDs {
		return invalid("monitor_order must be an array containing at most 10000 IDs")
	}
	seen := make(map[int64]bool, len(order.MonitorOrder))
	for _, id := range order.MonitorOrder {
		if id <= 0 || seen[id] {
			return invalid("monitor_order must contain unique positive IDs")
		}
		seen[id] = true
	}
	return nil
}

func (s *ChannelMonitorService) GetDisplayOrder(ctx context.Context) (ChannelMonitorDisplayOrder, error) {
	if s.displayOrderStore == nil {
		return DefaultChannelMonitorDisplayOrder(), nil
	}
	raw, err := s.displayOrderStore.GetValue(ctx, SettingKeyChannelMonitorDisplayOrder)
	if errors.Is(err, ErrSettingNotFound) || (err == nil && raw == "") {
		return DefaultChannelMonitorDisplayOrder(), nil
	}
	if err != nil {
		return ChannelMonitorDisplayOrder{}, fmt.Errorf("read channel monitor display order: %w", err)
	}
	var order ChannelMonitorDisplayOrder
	if err := json.Unmarshal([]byte(raw), &order); err != nil {
		return ChannelMonitorDisplayOrder{}, fmt.Errorf("decode channel monitor display order: %w", err)
	}
	if err := validateChannelMonitorDisplayOrder(order); err != nil {
		return ChannelMonitorDisplayOrder{}, fmt.Errorf("invalid saved channel monitor display order: %w", err)
	}
	return order, nil
}

func (s *ChannelMonitorService) UpdateDisplayOrder(ctx context.Context, order ChannelMonitorDisplayOrder) error {
	if err := validateChannelMonitorDisplayOrder(order); err != nil {
		return err
	}
	if len(order.MonitorOrder) > 0 {
		ids, err := s.repo.ExistingIDs(ctx, order.MonitorOrder)
		if err != nil {
			return fmt.Errorf("validate channel monitor display order IDs: %w", err)
		}
		if len(ids) != len(order.MonitorOrder) {
			return infraerrors.BadRequest("CHANNEL_MONITOR_DISPLAY_ORDER_STALE", "Some monitors no longer exist; refresh the monitor list before saving the order")
		}
	}
	if s.displayOrderStore == nil {
		return errors.New("channel monitor display order storage is unavailable")
	}
	value, err := json.Marshal(order)
	if err != nil {
		return fmt.Errorf("encode channel monitor display order: %w", err)
	}
	if err := s.displayOrderStore.Set(ctx, SettingKeyChannelMonitorDisplayOrder, string(value)); err != nil {
		return fmt.Errorf("save channel monitor display order: %w", err)
	}
	return nil
}

func channelMonitorDisplayGroup(provider string) string {
	if provider == MonitorProviderOpenAI || provider == MonitorProviderAnthropic {
		return provider
	}
	return "other"
}

// applyChannelMonitorDisplayOrder never exposes disabled or deleted monitor IDs.
// The returned monitor order includes newly-created visible monitors as well.
func applyChannelMonitorDisplayOrder(views []*UserMonitorView, order ChannelMonitorDisplayOrder) ChannelMonitorDisplayOrder {
	groups := make(map[string]int, len(order.GroupOrder))
	for rank, group := range order.GroupOrder {
		groups[group] = rank
	}
	positions := make(map[int64]int, len(order.MonitorOrder))
	for rank, id := range order.MonitorOrder {
		positions[id] = rank
	}
	sort.SliceStable(views, func(i, j int) bool {
		left, right := views[i], views[j]
		leftGroup, rightGroup := groups[channelMonitorDisplayGroup(left.Provider)], groups[channelMonitorDisplayGroup(right.Provider)]
		if leftGroup != rightGroup {
			return leftGroup < rightGroup
		}
		leftRank, leftSaved := positions[left.ID]
		rightRank, rightSaved := positions[right.ID]
		if leftSaved != rightSaved {
			return leftSaved
		}
		if leftSaved && leftRank != rightRank {
			return leftRank < rightRank
		}
		return left.ID < right.ID
	})
	visibleOrder := ChannelMonitorDisplayOrder{
		GroupOrder:   append([]string{}, order.GroupOrder...),
		MonitorOrder: make([]int64, 0, len(views)),
	}
	for _, view := range views {
		visibleOrder.MonitorOrder = append(visibleOrder.MonitorOrder, view.ID)
	}
	return visibleOrder
}

func (s *ChannelMonitorService) ListUserViewWithDisplayOrder(ctx context.Context) ([]*UserMonitorView, ChannelMonitorDisplayOrder, error) {
	order, err := s.GetDisplayOrder(ctx)
	if err != nil {
		return nil, ChannelMonitorDisplayOrder{}, err
	}
	views, err := s.listUserView(ctx)
	if err != nil {
		return nil, ChannelMonitorDisplayOrder{}, err
	}
	return views, applyChannelMonitorDisplayOrder(views, order), nil
}
