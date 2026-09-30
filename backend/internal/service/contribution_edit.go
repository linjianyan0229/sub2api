package service

import "context"

// 贡献者仅可修改连接与容量配置，不能覆盖审核、分组绑定和账号运行状态。
type ContributionAccountInput struct {
	SiteName       string   `json:"site_name"`
	BaseURL        string   `json:"base_url"`
	APIKey         string   `json:"api_key,omitempty"`
	RPM            int      `json:"rpm"`
	Concurrency    int      `json:"concurrency"`
	DailyLimit     float64  `json:"daily_limit"`
	TotalLimit     float64  `json:"total_limit"`
	Models         []string `json:"models"`
	ShowInStatus   bool     `json:"show_in_status"`
	MonitorModel   string   `json:"monitor_model,omitempty"`
	MonitorAPIMode string   `json:"monitor_api_mode,omitempty"`
}

type ContributionAccountView struct {
	ContributionAccountInput
	Platform string `json:"platform"`
}

type contributionEditor interface {
	GetOwnedAccount(context.Context, int64, int64) (*ContributionAccountView, error)
	UpdateOwnedAccount(context.Context, int64, int64, ContributionAccountInput, string) error
}

func (s *ChannelContributionService) OwnedAccount(ctx context.Context, actor, id int64) (*ContributionAccountView, error) {
	return s.repo.(contributionEditor).GetOwnedAccount(ctx, actor, id)
}

func (s *ChannelContributionService) UpdateOwnedAccount(ctx context.Context, actor, id int64, in ContributionAccountInput) error {
	config := ChannelContributionInput{SiteName: in.SiteName, BaseURL: in.BaseURL, APIKey: in.APIKey,
		ContributionSettings: ContributionSettings{RPM: in.RPM, Concurrency: in.Concurrency, DailyLimit: in.DailyLimit, TotalLimit: in.TotalLimit, Models: in.Models}}
	if err := validateContributionConnection(&config, false); err != nil {
		return err
	}
	in.SiteName, in.BaseURL, in.APIKey = config.SiteName, config.BaseURL, config.APIKey
	in.RPM, in.Concurrency, in.DailyLimit, in.TotalLimit, in.Models = config.RPM, config.Concurrency, config.DailyLimit, config.TotalLimit, config.Models
	if err := validateContributionMonitor(&in); err != nil {
		return err
	}
	tz := s.timezone
	if tz == "" {
		tz = "UTC"
	}
	if err := s.repo.(contributionEditor).UpdateOwnedAccount(ctx, actor, id, in, tz); err != nil {
		return err
	}
	s.scheduleContributionMonitor(ctx, id)
	return nil
}
