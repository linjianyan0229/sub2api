package service

import (
	"context"
	"net/url"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/util/urlvalidator"
)

var (
	ErrContributionNotFound = infraerrors.NotFound("CONTRIBUTION_NOT_FOUND", "渠道申请不存在")
	ErrContributionReviewed = infraerrors.Conflict("CONTRIBUTION_REVIEWED", "该申请已审核，请刷新列表")
	ErrContributionGroup    = infraerrors.BadRequest("CONTRIBUTION_GROUP_INVALID", "分组不存在、不可用或接口类型不匹配")
	ErrContributionAdmin    = infraerrors.Forbidden("CONTRIBUTION_ADMIN_REQUIRED", "仅管理员可审核渠道")
	ErrContributionLimit    = infraerrors.TooManyRequests("CONTRIBUTION_LIMIT", "待审核申请已达 20 条，请等待审核")
)

type ChannelContribution struct {
	ID                 int64      `json:"id"`
	UserID             *int64     `json:"user_id,omitempty"`
	ContributorName    string     `json:"contributor_name"`
	SiteName           string     `json:"site_name"`
	BaseURL            string     `json:"base_url"`
	Platform           string     `json:"platform"`
	KeyEncrypted       string     `json:"-"`
	RequestedGroupID   *int64     `json:"requested_group_id"`
	NewGroupName       string     `json:"new_group_name"`
	RequestedGroupName string     `json:"requested_group_name"`
	Status             string     `json:"status"`
	ReviewNote         string     `json:"review_note"`
	ReviewedAt         *time.Time `json:"reviewed_at"`
	GroupID            *int64     `json:"group_id"`
	AccountID          *int64     `json:"account_id"`
	CreatedAt          time.Time  `json:"created_at"`
}

type GroupContribution struct {
	AccountID       int64  `json:"account_id"`
	AccountName     string `json:"account_name"`
	ContributorName string `json:"contributor_name"`
}

type ChannelContributionInput struct {
	SiteName     string `json:"site_name"`
	BaseURL      string `json:"base_url"`
	APIKey       string `json:"api_key"`
	Platform     string `json:"platform"`
	GroupID      *int64 `json:"group_id"`
	NewGroupName string `json:"new_group_name"`
}

type ChannelContributionRepository interface {
	Submit(context.Context, int64, *ChannelContribution) error
	List(context.Context, int64, bool, string, int64) ([]ChannelContribution, error)
	GetForReview(context.Context, int64, int64) (*ChannelContribution, error)
	Review(context.Context, int64, *ChannelContribution, bool, string, *Account) error
	Contributors(context.Context, []int64) (map[int64][]GroupContribution, error)
}

type ChannelContributionService struct {
	repo      ChannelContributionRepository
	encryptor SecretEncryptor
	keys      *APIKeyService
}

func NewChannelContributionService(repo ChannelContributionRepository, encryptor SecretEncryptor, keys *APIKeyService) *ChannelContributionService {
	return &ChannelContributionService{repo: repo, encryptor: encryptor, keys: keys}
}

func contributionPlatform(platform string) bool {
	return platform == PlatformOpenAI || platform == PlatformAnthropic || platform == PlatformGemini
}

func validateContribution(in *ChannelContributionInput) error {
	in.SiteName, in.NewGroupName = strings.TrimSpace(in.SiteName), strings.TrimSpace(in.NewGroupName)
	in.APIKey = strings.TrimSpace(in.APIKey)
	if in.Platform == "" {
		in.Platform = PlatformOpenAI
	}
	validName := func(s string) bool {
		return utf8.ValidString(s) && utf8.RuneCountInString(s) <= 100 && strings.IndexFunc(s, unicode.IsControl) < 0
	}
	if in.SiteName == "" || !validName(in.SiteName) || !validName(in.NewGroupName) || !contributionPlatform(in.Platform) {
		return infraerrors.BadRequest("CONTRIBUTION_INVALID", "请填写有效的站点名称和接口类型")
	}
	if (in.GroupID == nil) == (in.NewGroupName == "") || (in.GroupID != nil && *in.GroupID <= 0) {
		return ErrContributionGroup
	}
	if in.APIKey == "" || len(in.APIKey) > 4096 || strings.IndexFunc(in.APIKey, unicode.IsSpace) >= 0 || strings.IndexFunc(in.APIKey, unicode.IsControl) >= 0 {
		return infraerrors.BadRequest("CONTRIBUTION_KEY_INVALID", "请填写有效的 API Key")
	}
	normalized, err := urlvalidator.ValidateHTTPSURL(in.BaseURL, urlvalidator.ValidationOptions{})
	parsed, parseErr := url.Parse(normalized)
	if err != nil || parseErr != nil || len(normalized) > 2048 || parsed == nil || parsed.User != nil || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" {
		return infraerrors.BadRequest("CONTRIBUTION_URL_INVALID", "请填写公网 HTTPS API 地址，不可包含账号、查询参数或锚点")
	}
	in.BaseURL = normalized
	return nil
}

func (s *ChannelContributionService) Groups(ctx context.Context, user int64) ([]Group, error) {
	groups, err := s.keys.GetAvailableGroups(ctx, user)
	if err != nil {
		return nil, err
	}
	out := []Group{}
	for _, g := range groups {
		if contributionPlatform(g.Platform) {
			out = append(out, g)
		}
	}
	return out, nil
}

func (s *ChannelContributionService) Submit(ctx context.Context, user int64, in ChannelContributionInput) (*ChannelContribution, error) {
	if err := validateContribution(&in); err != nil {
		return nil, err
	}
	if in.GroupID != nil {
		groups, err := s.Groups(ctx, user)
		if err != nil {
			return nil, err
		}
		allowed := false
		for _, g := range groups {
			if g.ID == *in.GroupID && g.Platform == in.Platform {
				allowed = true
				break
			}
		}
		if !allowed {
			return nil, ErrContributionGroup
		}
	}
	secret, err := s.encryptor.Encrypt(in.APIKey)
	if err != nil {
		return nil, infraerrors.InternalServer("CONTRIBUTION_ENCRYPT_FAILED", "暂时无法保存 Key")
	}
	c := &ChannelContribution{SiteName: in.SiteName, BaseURL: in.BaseURL, Platform: in.Platform, KeyEncrypted: secret, RequestedGroupID: in.GroupID, NewGroupName: in.NewGroupName}
	if err := s.repo.Submit(ctx, user, c); err != nil {
		return nil, err
	}
	return c, nil
}

func (s *ChannelContributionService) List(ctx context.Context, user int64, admin bool, status string, before int64) ([]ChannelContribution, error) {
	if status != "" && status != "pending" && status != "approved" && status != "rejected" {
		return nil, infraerrors.BadRequest("CONTRIBUTION_STATUS_INVALID", "无效的审核状态")
	}
	return s.repo.List(ctx, user, admin, status, before)
}

func (s *ChannelContributionService) Review(ctx context.Context, actor, id int64, approve bool, note string) error {
	note = strings.TrimSpace(note)
	if !utf8.ValidString(note) || utf8.RuneCountInString(note) > 500 {
		return infraerrors.BadRequest("CONTRIBUTION_NOTE_INVALID", "审核备注最多 500 字")
	}
	c, err := s.repo.GetForReview(ctx, actor, id)
	if err != nil {
		return err
	}
	if c.Status != "pending" {
		return ErrContributionReviewed
	}
	var account *Account
	if approve {
		key, err := s.encryptor.Decrypt(c.KeyEncrypted)
		if err != nil {
			return infraerrors.InternalServer("CONTRIBUTION_DECRYPT_FAILED", "无法读取申请 Key")
		}
		// 复用账号默认值构建；仅由服务端指定类型、倍率与调度字段。
		account, err = buildAccountForCreate(&CreateAccountInput{Name: c.SiteName, Platform: c.Platform, Type: AccountTypeAPIKey, Concurrency: 5, Priority: 50, Credentials: map[string]any{"base_url": c.BaseURL, "api_key": key}}, map[string]any{"contribution_id": c.ID})
		if err != nil {
			return err
		}
	}
	return s.repo.Review(ctx, actor, c, approve, note, account)
}

func (s *ChannelContributionService) EnrichGroups(ctx context.Context, groups []Group) error {
	if s == nil || len(groups) == 0 {
		return nil
	}
	ids := make([]int64, len(groups))
	for i := range groups {
		ids[i] = groups[i].ID
	}
	byGroup, err := s.repo.Contributors(ctx, ids)
	if err != nil {
		return err
	}
	for i := range groups {
		groups[i].Contributions = byGroup[groups[i].ID]
	}
	return nil
}
