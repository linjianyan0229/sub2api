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
		Quotas(context.Context, int64, bool, []int64, int64) ([]ContributionQuotaSource, error)
	})
	rows, err := source.Quotas(ctx, userID, user.IsAdmin(), ids, before)
	if err != nil {
		return nil, nil, err
	}
	var shared *SharedQuotaSnapshot
	if s.keys.sharedQuota != nil {
		shared, err = s.keys.sharedQuota.currentSnapshot(ctx, 0)
		if err != nil {
			return nil, nil, err
		}
	}
	views := make([]ContributionQuotaView, 0, len(rows))
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
				case a.IsQuotaExceeded():
					v.Status = "quota_exhausted"
				case !a.IsSchedulable():
					v.Status = "limited"
				default:
					v.Status = "active"
				}
			}
		}
		v.Independent = v.DailyLimit > 0 || v.TotalLimit > 0
		if v.Models == nil {
			v.Models = []string{}
		}
		if v.Groups == nil {
			v.Groups = []ContributionGroupView{}
		}
		views = append(views, v)
	}
	return views, shared, nil
}
