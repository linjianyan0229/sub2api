package service

import "context"

// 独立额度仅用于已审核的贡献账号；客户端不能通过请求参数开启。
func (a *Account) HasIndependentContributionQuota() bool {
	return a != nil && a.getExtraInt("contribution_id") > 0 && (a.GetQuotaDailyLimit() > 0 || a.GetQuotaLimit() > 0)
}

type ContributionAdmissionRepository interface {
	AdmitContributionAccount(context.Context, int64, *SharedQuotaSnapshot) (bool, error)
}

func sharedQuotaAccountAllowed(ctx context.Context, a *Account) bool {
	q := SharedQuotaFromContext(ctx)
	if q == nil || q.CheckPoolRemaining() == nil {
		return true
	}
	if !a.HasIndependentContributionQuota() {
		return false
	}
	for _, id := range q.IndependentAccountIDs {
		if id == a.ID {
			return true
		}
	}
	return false
}

func filterSharedQuotaAccounts(ctx context.Context, accounts []Account) []Account {
	q := SharedQuotaFromContext(ctx)
	if q == nil || q.CheckPoolRemaining() == nil {
		return accounts
	}
	out := make([]Account, 0, len(accounts))
	for i := range accounts {
		if sharedQuotaAccountAllowed(ctx, &accounts[i]) {
			out = append(out, accounts[i])
		}
	}
	return out
}
