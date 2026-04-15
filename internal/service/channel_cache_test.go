package service

import (
	"testing"
	"time"

	"ampmanager/internal/model"
)

type countingChannelRepo struct {
	channels          map[string]*model.Channel
	listEnabledCalls  int
	channelGroupIDs   map[string][]string
	groupBatchCalls   int
	getByIDCalls      int
	getGroupIDCalls   int
}

func (r *countingChannelRepo) Create(channel *model.Channel) error {
	r.channels[channel.ID] = channel
	return nil
}

func (r *countingChannelRepo) GetByID(id string) (*model.Channel, error) {
	r.getByIDCalls++
	channel, ok := r.channels[id]
	if !ok {
		return nil, nil
	}
	copyChannel := *channel
	return &copyChannel, nil
}

func (r *countingChannelRepo) List() ([]*model.Channel, error) { return nil, nil }

func (r *countingChannelRepo) ListEnabled() ([]*model.Channel, error) {
	r.listEnabledCalls++
	var channels []*model.Channel
	for _, channel := range r.channels {
		if !channel.Enabled {
			continue
		}
		copyChannel := *channel
		channels = append(channels, &copyChannel)
	}
	return channels, nil
}

func (r *countingChannelRepo) Update(channel *model.Channel) error {
	r.channels[channel.ID] = channel
	return nil
}

func (r *countingChannelRepo) Delete(id string) error {
	delete(r.channels, id)
	return nil
}

func (r *countingChannelRepo) SetEnabled(id string, enabled bool) error {
	if channel, ok := r.channels[id]; ok {
		channel.Enabled = enabled
	}
	return nil
}

func (r *countingChannelRepo) SetGroups(id string, groupIDs []string) error {
	if r.channelGroupIDs == nil {
		r.channelGroupIDs = make(map[string][]string)
	}
	r.channelGroupIDs[id] = append([]string(nil), groupIDs...)
	return nil
}

func (r *countingChannelRepo) GetGroupIDs(channelID string) ([]string, error) {
	r.getGroupIDCalls++
	return append([]string(nil), r.channelGroupIDs[channelID]...), nil
}

func (r *countingChannelRepo) GetGroupIDsByChannelIDs(channelIDs []string) (map[string][]string, error) {
	r.groupBatchCalls++
	result := make(map[string][]string, len(channelIDs))
	for _, channelID := range channelIDs {
		result[channelID] = append([]string(nil), r.channelGroupIDs[channelID]...)
	}
	return result, nil
}

func TestSelectChannelForModelUsesEnabledChannelCache(t *testing.T) {
	invalidateEnabledChannelsCache()
	repo := &countingChannelRepo{
		channels: map[string]*model.Channel{
			"ch-1": {
				ID:         "ch-1",
				Enabled:    true,
				Priority:   1,
				Weight:     1,
				Type:       model.ChannelTypeOpenAI,
				ModelsJSON: `[{"name":"gpt-4o"}]`,
			},
		},
	}
	svc := NewChannelServiceWithRepo(repo)

	if _, err := svc.SelectChannelForModel("gpt-4o"); err != nil {
		t.Fatalf("SelectChannelForModel returned error: %v", err)
	}
	if _, err := svc.SelectChannelForModel("gpt-4o"); err != nil {
		t.Fatalf("second SelectChannelForModel returned error: %v", err)
	}
	if repo.listEnabledCalls != 1 {
		t.Fatalf("ListEnabled calls = %d, want 1", repo.listEnabledCalls)
	}
}

func TestEnabledChannelCacheInvalidatesOnSetEnabled(t *testing.T) {
	invalidateEnabledChannelsCache()
	repo := &countingChannelRepo{
		channels: map[string]*model.Channel{
			"ch-1": {
				ID:         "ch-1",
				Enabled:    true,
				Priority:   1,
				Weight:     1,
				Type:       model.ChannelTypeOpenAI,
				ModelsJSON: `[{"name":"gpt-4o"}]`,
			},
		},
	}
	svc := NewChannelServiceWithRepo(repo)

	if _, err := svc.SelectChannelForModel("gpt-4o"); err != nil {
		t.Fatalf("SelectChannelForModel returned error: %v", err)
	}
	if repo.listEnabledCalls != 1 {
		t.Fatalf("ListEnabled calls = %d, want 1", repo.listEnabledCalls)
	}
	if err := svc.SetEnabled("ch-1", false); err != nil {
		t.Fatalf("SetEnabled returned error: %v", err)
	}
	if _, err := svc.SelectChannelForModel("gpt-4o"); err != nil {
		t.Fatalf("SelectChannelForModel after invalidate returned error: %v", err)
	}
	if repo.listEnabledCalls != 2 {
		t.Fatalf("ListEnabled calls = %d, want 2", repo.listEnabledCalls)
	}
}

func TestEnabledChannelCacheExpiresByTTL(t *testing.T) {
	invalidateEnabledChannelsCache()
	previousTTL := enabledChannelsSnapshot.cacheTTL
	enabledChannelsSnapshot.cacheTTL = time.Millisecond
	defer func() { enabledChannelsSnapshot.cacheTTL = previousTTL }()

	repo := &countingChannelRepo{
		channels: map[string]*model.Channel{
			"ch-1": {
				ID:         "ch-1",
				Enabled:    true,
				Priority:   1,
				Weight:     1,
				Type:       model.ChannelTypeOpenAI,
				ModelsJSON: `[{"name":"gpt-4o"}]`,
			},
		},
	}
	svc := NewChannelServiceWithRepo(repo)

	if _, err := svc.SelectChannelForModel("gpt-4o"); err != nil {
		t.Fatalf("SelectChannelForModel returned error: %v", err)
	}
	time.Sleep(2 * time.Millisecond)
	if _, err := svc.SelectChannelForModel("gpt-4o"); err != nil {
		t.Fatalf("SelectChannelForModel after ttl returned error: %v", err)
	}
	if repo.listEnabledCalls != 2 {
		t.Fatalf("ListEnabled calls = %d, want 2", repo.listEnabledCalls)
	}
}

func TestSelectChannelForModelWithGroupsUsesCachedGroupSnapshot(t *testing.T) {
	invalidateEnabledChannelsCache()
	repo := &countingChannelRepo{
		channels: map[string]*model.Channel{
			"ch-1": {
				ID:         "ch-1",
				Enabled:    true,
				Priority:   1,
				Weight:     1,
				Type:       model.ChannelTypeOpenAI,
				ModelsJSON: `[{"name":"gpt-4o"}]`,
			},
		},
		channelGroupIDs: map[string][]string{
			"ch-1": {"group-a"},
		},
	}
	svc := NewChannelServiceWithRepo(repo)

	if _, err := svc.SelectChannelForModelWithGroups("gpt-4o", []string{"group-a"}); err != nil {
		t.Fatalf("SelectChannelForModelWithGroups returned error: %v", err)
	}
	if _, err := svc.SelectChannelForModelWithGroups("gpt-4o", []string{"group-a"}); err != nil {
		t.Fatalf("second SelectChannelForModelWithGroups returned error: %v", err)
	}
	if repo.listEnabledCalls != 1 {
		t.Fatalf("ListEnabled calls = %d, want 1", repo.listEnabledCalls)
	}
	if repo.groupBatchCalls != 1 {
		t.Fatalf("GetGroupIDsByChannelIDs calls = %d, want 1", repo.groupBatchCalls)
	}
}

func TestSelectSpecificChannelForModelWithGroupsUsesCachedSnapshot(t *testing.T) {
	invalidateEnabledChannelsCache()
	repo := &countingChannelRepo{
		channels: map[string]*model.Channel{
			"ch-1": {
				ID:         "ch-1",
				Enabled:    true,
				Priority:   1,
				Weight:     1,
				Type:       model.ChannelTypeOpenAI,
				ModelsJSON: `[{"name":"gpt-4o"}]`,
			},
		},
		channelGroupIDs: map[string][]string{
			"ch-1": {"group-a"},
		},
	}
	svc := NewChannelServiceWithRepo(repo)

	if _, err := svc.SelectSpecificChannelForModelWithGroups("ch-1", "gpt-4o", []string{"group-a"}); err != nil {
		t.Fatalf("SelectSpecificChannelForModelWithGroups returned error: %v", err)
	}
	if _, err := svc.SelectSpecificChannelForModelWithGroups("ch-1", "gpt-4o", []string{"group-a"}); err != nil {
		t.Fatalf("second SelectSpecificChannelForModelWithGroups returned error: %v", err)
	}
	if repo.listEnabledCalls != 1 {
		t.Fatalf("ListEnabled calls = %d, want 1", repo.listEnabledCalls)
	}
	if repo.groupBatchCalls != 1 {
		t.Fatalf("GetGroupIDsByChannelIDs calls = %d, want 1", repo.groupBatchCalls)
	}
	if repo.getByIDCalls != 0 {
		t.Fatalf("GetByID calls = %d, want 0", repo.getByIDCalls)
	}
	if repo.getGroupIDCalls != 0 {
		t.Fatalf("GetGroupIDs calls = %d, want 0", repo.getGroupIDCalls)
	}
}
