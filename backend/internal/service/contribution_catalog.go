package service

import (
	"context"
	"sort"
)

type ContributionGroupView struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type ContributionQuotaSource struct {
	ID                                          int64
	SiteName, ContributorName, Platform, Status string
	Settings                                    ContributionSettings
	Groups                                      []ContributionGroupView
	Account                                     *Account
	QuotaAvailable                              bool
	RPMLimited                                  bool
}

type ContributionQuotaView struct {
	ID              int64                   `json:"id"`
	SiteName        string                  `json:"site_name"`
	ContributorName string                  `json:"contributor_name"`
	Platform        string                  `json:"platform"`
	Status          string                  `json:"status"`
	Groups          []ContributionGroupView `json:"groups"`
	RPM             int                     `json:"rpm"`
	Concurrency     int                     `json:"concurrency"`
	DailyLimit      float64                 `json:"daily_limit"`
	TotalLimit      float64                 `json:"total_limit"`
	DailyUsed       float64                 `json:"daily_used"`
	TotalUsed       float64                 `json:"total_used"`
	Models          []string                `json:"models"`
	Independent     bool                    `json:"independent"`
}

func (s *ChannelContributionService) Quotas(ctx context.Context, userID, before int64) ([]ContributionQuotaView, *SharedQuotaSnapshot, error) {
	return s.contributionCatalog(ctx, userID, before, false)
}

func (s *ChannelContributionService) Statuses(ctx context.Context, userID, before int64) ([]ContributionQuotaView, error) {
	items, _, err := s.contributionCatalog(ctx, userID, before, true)
	return items, err
}

func (s *ChannelContributionService) contributionCatalog(ctx context.Context, userID, before int64, statusPage bool) ([]ContributionQuotaView, *SharedQuotaSnapshot, error) {
	user, err := s.keys.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, nil, err
	}
	groups, err := s.Groups(ctx, userID)
	if err != nil {
		return nil, nil, err
	}
	ids := make([]int64, 0, len(groups))
	for _, group := range groups {
		ids = append(ids, group.ID)
	}
	source := s.repo.(interface {
		Catalog(context.Context, int64, bool, []int64, int64, bool) ([]ContributionQuotaSource, error)
	})
	var shared *SharedQuotaSnapshot
	if s.keys.sharedQuota != nil {
		shared, err = s.keys.sharedQuota.currentSnapshot(ctx, 0)
		if err != nil {
			return nil, nil, err
		}
	}
	views := make([]ContributionQuotaView, 0, 51)
	// 过滤后凑齐分页，不可用渠道不能占用名额或截断更早的可用渠道。
	for len(views) < 51 {
		rows, err := source.Catalog(ctx, userID, user.IsAdmin(), ids, before, statusPage)
		if err != nil {
			return nil, nil, err
		}
		for _, row := range rows {
			v := ContributionQuotaView{ID: row.ID, SiteName: row.SiteName, ContributorName: row.ContributorName, Platform: row.Platform, Status: row.Status, Groups: row.Groups, RPM: row.Settings.RPM, Concurrency: row.Settings.Concurrency, DailyLimit: row.Settings.DailyLimit, TotalLimit: row.Settings.TotalLimit, Models: row.Settings.Models}
			if v.Concurrency == 0 {
				v.Concurrency = 5
			}
			if row.Status == "approved" {
				a := row.Account
				if a == nil {
					v.Status = "removed"
				} else {
					v.SiteName, v.Platform, v.RPM, v.Concurrency = a.Name, a.Platform, a.GetBaseRPM(), a.Concurrency
					v.DailyLimit, v.TotalLimit, v.DailyUsed, v.TotalUsed = a.GetQuotaDailyLimit(), a.GetQuotaLimit(), a.GetQuotaDailyUsed(), a.GetQuotaUsed()
					if a.IsDailyQuotaPeriodExpired() {
						v.DailyUsed = 0
					}
					v.Models = []string{}
					for model := range a.GetModelMapping() {
						v.Models = append(v.Models, model)
					}
					sort.Strings(v.Models)
					switch {
					case a.Status != StatusActive:
						v.Status = a.Status
					case !a.Schedulable:
						v.Status = "disabled"
					case a.IsQuotaExceeded() || !row.QuotaAvailable:
						v.Status = "quota_exhausted"
					case !a.IsSchedulable() || row.RPMLimited:
						v.Status = "limited"
					default:
						v.Status = "active"
					}
				}
			}
			v.Independent = v.DailyLimit > 0 || v.TotalLimit > 0
			if v.Status == "active" && !v.Independent && shared != nil && shared.CheckPoolRemaining() != nil {
				v.Status = "quota_exhausted"
			}
			if !statusPage && v.Status != "active" {
				continue
			}
			if v.Models == nil {
				v.Models = []string{}
			}
			if v.Groups == nil {
				v.Groups = []ContributionGroupView{}
			}
			views = append(views, v)
			if len(views) == 51 {
				break
			}
		}
		if len(rows) < 51 {
			break
		}
		before = rows[len(rows)-1].ID
	}
	return views, shared, nil
}
