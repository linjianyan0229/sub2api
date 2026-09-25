package service

import (
	"context"
	"errors"
	"strings"
)

// UsageLeaderboardItem 仅公开昵称和用量，不包含邮箱、用户 ID 或密钥。
type UsageLeaderboardItem struct {
	Username   string  `json:"username"`
	ActualCost float64 `json:"actual_cost"`
	Tokens     int64   `json:"tokens"`
}

type UsageLeaderboardRepository interface {
	GetUsageLeaderboard(context.Context) ([]UsageLeaderboardItem, error)
}

func (s *UsageService) GetUsageLeaderboard(ctx context.Context) ([]UsageLeaderboardItem, error) {
	repo, ok := s.usageRepo.(UsageLeaderboardRepository)
	if !ok {
		return nil, errors.New("用量排行榜不可用")
	}
	items, err := repo.GetUsageLeaderboard(ctx)
	if err != nil {
		return nil, err
	}
	if items == nil {
		items = []UsageLeaderboardItem{}
	}
	for i := range items {
		items[i].Username = strings.TrimSpace(items[i].Username)
		// 部分 OAuth 账号以邮箱作为初始昵称，不能向其他用户公开。
		if strings.Contains(items[i].Username, "@") {
			items[i].Username = ""
		}
	}
	return items, nil
}
