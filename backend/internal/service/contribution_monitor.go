package service

import (
	"context"
	"log/slog"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const ChannelMonitorContributionMetadataKey = "sub2api:contribution_id"

type contributionMonitorRepository interface {
	ContributionMonitors(context.Context, []int64) ([]*ChannelMonitor, error)
	ContributionProbeAccount(context.Context, int64) (*Account, error)
	SyncContributionMonitors(context.Context) error
}

type ContributionMonitorView struct {
	ID              int64
	ContributorName string
	Monitor         *UserMonitorView
	Probing         bool
}

func validateContributionMonitor(in *ContributionAccountInput) error {
	in.MonitorModel = strings.TrimSpace(in.MonitorModel)
	if in.MonitorAPIMode == "" {
		in.MonitorAPIMode = MonitorAPIModeChatCompletions
	}
	if in.MonitorAPIMode != MonitorAPIModeChatCompletions && in.MonitorAPIMode != MonitorAPIModeResponses {
		return infraerrors.BadRequest("CONTRIBUTION_MONITOR_INVALID", "请选择有效的检测协议")
	}
	if !in.ShowInStatus {
		return nil
	}
	if in.MonitorModel == "" && len(in.Models) > 0 {
		in.MonitorModel = in.Models[0]
	}
	if in.MonitorModel == "" {
		return infraerrors.BadRequest("CONTRIBUTION_MONITOR_INVALID", "请选择检测模型")
	}
	settings := ContributionSettings{Models: []string{in.MonitorModel}}
	if err := validateContributionSettings(&settings); err != nil {
		return err
	}
	if len(in.Models) > 0 {
		found := false
		for _, model := range in.Models {
			if model == in.MonitorModel {
				found = true
				break
			}
		}
		if !found {
			return infraerrors.BadRequest("CONTRIBUTION_MONITOR_INVALID", "检测模型须在渠道模型中")
		}
	}
	return nil
}

// 在 runner 启动前注入，避免贡献服务和调度器的构造依赖环。
func (s *ChannelContributionService) SetMonitorService(monitors *ChannelMonitorService) {
	s.monitors = monitors
	if monitors != nil {
		monitors.contributions, _ = s.repo.(contributionMonitorRepository)
	}
}

func (s *ChannelContributionService) InitializeMonitors(ctx context.Context) error {
	if repo, ok := s.repo.(contributionMonitorRepository); ok {
		return repo.SyncContributionMonitors(ctx)
	}
	return nil
}

func (s *ChannelContributionService) scheduleContributionMonitor(ctx context.Context, id int64) {
	if s.monitors == nil || s.monitors.scheduler == nil {
		return
	}
	items, err := s.repo.(contributionMonitorRepository).ContributionMonitors(ctx, []int64{id})
	if err != nil {
		slog.Error("contribution monitor schedule failed", "contribution_id", id, "error", err)
		return
	}
	for _, m := range items {
		s.monitors.scheduler.Schedule(m)
	}
}

func (s *ChannelContributionService) MonitorViews(ctx context.Context, actor, before int64) ([]ContributionMonitorView, error) {
	out := []ContributionMonitorView{}
	if s.monitors == nil {
		return out, nil
	}
	runtime := s.monitors.probeRuntime(ctx)
	if !runtime.Enabled {
		return out, nil
	}
	visible, err := s.Statuses(ctx, actor, before)
	if err != nil {
		return nil, err
	}
	ids := make([]int64, 0, len(visible))
	for _, item := range visible {
		ids = append(ids, item.ID)
	}
	monitors, err := s.repo.(contributionMonitorRepository).ContributionMonitors(ctx, ids)
	if err != nil {
		return nil, err
	}
	byID := map[int64]*ChannelMonitor{}
	views := map[int64]*UserMonitorView{}
	for i, v := range s.monitors.buildUserViews(ctx, monitors) {
		byID[monitors[i].ContributionID] = monitors[i]
		views[monitors[i].ContributionID] = v
	}
	for _, item := range visible {
		v := views[item.ID]
		if v == nil {
			v = &UserMonitorView{Provider: item.Platform}
		}
		v.Name = item.SiteName
		names := make([]string, 0, len(item.Groups))
		for _, g := range item.Groups {
			names = append(names, g.Name)
		}
		v.GroupName = strings.Join(names, " / ")
		m := byID[item.ID]
		probing := m != nil && m.Enabled && runtime.ActiveProbesAllowed() && item.Status == "active"
		out = append(out, ContributionMonitorView{ID: item.ID, ContributorName: item.ContributorName, Monitor: v, Probing: probing})
	}
	return out, nil
}

func (s *ChannelContributionService) MonitorDetail(ctx context.Context, actor, id int64) (*UserMonitorDetail, error) {
	if id <= 0 || id == int64(^uint64(0)>>1) {
		return nil, ErrChannelMonitorNotFound
	}
	// 与列表共用分组权限过滤，不能通过猜测监控或贡献 ID 读取私有渠道。
	items, err := s.MonitorViews(ctx, actor, id+1)
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		if item.ID != id {
			continue
		}
		if item.Monitor.ID == 0 {
			return &UserMonitorDetail{Name: item.Monitor.Name, Provider: item.Monitor.Provider, GroupName: item.Monitor.GroupName}, nil
		}
		m, err := s.monitors.repo.GetByID(ctx, item.Monitor.ID)
		if err != nil {
			return nil, err
		}
		m.Name, m.GroupName = item.Monitor.Name, item.Monitor.GroupName
		return s.monitors.userDetail(ctx, m)
	}
	return nil, ErrChannelMonitorNotFound
}

func (s *ChannelMonitorService) prepareContributionProbe(ctx context.Context, m *ChannelMonitor) error {
	if s.contributions == nil || !m.Enabled {
		return ErrChannelMonitorNotFound
	}
	a, err := s.contributions.ContributionProbeAccount(ctx, m.ContributionID)
	if err != nil {
		return err
	}
	if m.AccountID == nil || a.ID != *m.AccountID || a.Type != AccountTypeAPIKey || !contributionPlatform(a.Platform) || a.Status != StatusActive || !a.Schedulable || (a.AutoPauseOnExpired && a.ExpiresAt != nil && !a.ExpiresAt.After(time.Now())) {
		return ErrChannelMonitorDisabled
	}
	in := ChannelContributionInput{SiteName: a.Name, BaseURL: a.GetCredential("base_url"), APIKey: a.GetCredential("api_key")}
	if err := validateContributionConnection(&in, true); err != nil {
		return err
	}
	model := m.PrimaryModel
	if mapping := a.GetModelMapping(); len(mapping) > 0 {
		target, exists := mapping[model]
		if !exists {
			return infraerrors.BadRequest("CONTRIBUTION_MONITOR_INVALID", "检测模型已移除")
		}
		model = target
	}
	m.Provider, m.Endpoint, m.APIKey, m.ProbeModel = a.Platform, in.BaseURL, in.APIKey, model
	return nil
}
