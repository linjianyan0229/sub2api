package service

import (
	"context"
	"math"
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
	ContributionSettings
	PreparedGroup      *Group     `json:"-"`
	CopyAccountIDs     []int64    `json:"-"`
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
	CanEdit            bool       `json:"can_edit"`
}

type GroupContribution struct {
	AccountID       int64  `json:"account_id"`
	AccountName     string `json:"account_name"`
	ContributorName string `json:"contributor_name"`
}

type ChannelContributionInput struct {
	ContributionSettings
	SiteName     string `json:"site_name"`
	BaseURL      string `json:"base_url"`
	APIKey       string `json:"api_key"`
	Platform     string `json:"platform"`
	GroupID      *int64 `json:"group_id"`
	NewGroupName string `json:"new_group_name"`
}

type ContributionSettings struct {
	NewGroupConfig *CreateGroupInput `json:"new_group_config,omitempty"`
	RPM            int               `json:"rpm"`
	Concurrency    int               `json:"concurrency"`
	DailyLimit     float64           `json:"daily_limit"`
	TotalLimit     float64           `json:"total_limit"`
	Models         []string          `json:"models"`
	ShowInStatus   bool              `json:"show_in_status"`
}

func validateContributionSettings(s *ContributionSettings) error {
	if s.Concurrency == 0 {
		s.Concurrency = 5
	}
	if s.RPM < 0 || s.RPM > 100000 || s.Concurrency < 1 || s.Concurrency > 1000 {
		return infraerrors.BadRequest("CONTRIBUTION_CAPACITY_INVALID", "RPM 须为 0–100000，并发须为 1–1000")
	}
	for _, amount := range []float64{s.DailyLimit, s.TotalLimit} {
		if math.IsNaN(amount) || math.IsInf(amount, 0) || amount < 0 || amount > 1000000000 || (amount > 0 && QuantizeUsageBillingAmount(amount) == 0) {
			return infraerrors.BadRequest("CONTRIBUTION_QUOTA_INVALID", "金额限制须为 0–1000000000 USD")
		}
	}
	s.DailyLimit, s.TotalLimit = QuantizeUsageBillingAmount(s.DailyLimit), QuantizeUsageBillingAmount(s.TotalLimit)
	if len(s.Models) > 200 {
		return infraerrors.BadRequest("CONTRIBUTION_MODELS_INVALID", "最多选择 200 个模型")
	}
	models, seen := []string{}, map[string]bool{}
	for _, raw := range s.Models {
		model := strings.TrimSpace(raw)
		if model == "" || len(model) > 200 || !utf8.ValidString(model) || strings.ContainsAny(model, "*?") || strings.IndexFunc(model, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) >= 0 {
			return infraerrors.BadRequest("CONTRIBUTION_MODELS_INVALID", "请填写有效的模型 ID")
		}
		if !seen[model] {
			models = append(models, model)
			seen[model] = true
		}
	}
	s.Models = models
	return nil
}

type ChannelContributionRepository interface {
	Submit(context.Context, int64, *ChannelContribution) error
	List(context.Context, int64, bool, string, int64) ([]ChannelContribution, error)
	GetForReview(context.Context, int64, int64) (*ChannelContribution, error)
	Review(context.Context, int64, *ChannelContribution, bool, string, *Account) error
	Contributors(context.Context, []int64) (map[int64][]GroupContribution, error)
}

type ChannelContributionService struct {
	repo         ChannelContributionRepository
	encryptor    SecretEncryptor
	keys         *APIKeyService
	modelFetcher interface {
		FetchUpstreamSupportedModels(context.Context, *Account) ([]string, error)
	}
	timezone      string
	groupPreparer interface {
		PrepareGroup(context.Context, *CreateGroupInput) (*Group, []int64, error)
	}
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
	return validateContributionConnection(in, true)
}

func validateContributionConnection(in *ChannelContributionInput, requireKey bool) error {
	in.SiteName, in.APIKey = strings.TrimSpace(in.SiteName), strings.TrimSpace(in.APIKey)
	if in.SiteName == "" || !utf8.ValidString(in.SiteName) || utf8.RuneCountInString(in.SiteName) > 100 || strings.IndexFunc(in.SiteName, unicode.IsControl) >= 0 {
		return infraerrors.BadRequest("CONTRIBUTION_INVALID", "请填写有效的站点名称")
	}
	if (requireKey && in.APIKey == "") || len(in.APIKey) > 4096 || strings.IndexFunc(in.APIKey, unicode.IsSpace) >= 0 || strings.IndexFunc(in.APIKey, unicode.IsControl) >= 0 {
		return infraerrors.BadRequest("CONTRIBUTION_KEY_INVALID", "请填写有效的 API Key")
	}
	normalized, err := urlvalidator.ValidateHTTPSURL(in.BaseURL, urlvalidator.ValidationOptions{})
	parsed, parseErr := url.Parse(normalized)
	if err != nil || parseErr != nil || len(normalized) > 2048 || parsed == nil || parsed.User != nil || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" {
		return infraerrors.BadRequest("CONTRIBUTION_URL_INVALID", "请填写公网 HTTPS API 地址，不可包含账号、查询参数或锚点")
	}
	in.BaseURL = normalized
	return validateContributionSettings(&in.ContributionSettings)
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

type ContributionAccountOption struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

func (s *ChannelContributionService) GroupAccounts(ctx context.Context, groups []Group) (map[int64][]ContributionAccountOption, error) {
	source, ok := s.repo.(interface {
		GroupAccounts(context.Context, []int64) (map[int64][]ContributionAccountOption, error)
	})
	if !ok {
		return map[int64][]ContributionAccountOption{}, nil
	}
	ids := make([]int64, 0, len(groups))
	for _, group := range groups {
		ids = append(ids, group.ID)
	}
	return source.GroupAccounts(ctx, ids)
}

func (s *ChannelContributionService) Submit(ctx context.Context, user int64, in ChannelContributionInput) (*ChannelContribution, error) {
	if err := validateContribution(&in); err != nil {
		return nil, err
	}
	if in.NewGroupConfig != nil {
		if in.GroupID != nil {
			return nil, ErrContributionGroup
		}
		if err := s.prepareContributionGroup(ctx, user, in.NewGroupName, in.Platform, in.NewGroupConfig, nil); err != nil {
			return nil, err
		}
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
	c := &ChannelContribution{ContributionSettings: in.ContributionSettings, SiteName: in.SiteName, BaseURL: in.BaseURL, Platform: in.Platform, KeyEncrypted: secret, RequestedGroupID: in.GroupID, NewGroupName: in.NewGroupName}
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
	return s.ReviewWithModels(ctx, actor, id, approve, note, nil)
}

func (s *ChannelContributionService) ReviewWithModels(ctx context.Context, actor, id int64, approve bool, note string, models *[]string) error {
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
		if c.NewGroupConfig != nil {
			if c.UserID == nil {
				return ErrUserNotFound
			}
			if err := s.prepareContributionGroup(ctx, *c.UserID, c.NewGroupName, c.Platform, c.NewGroupConfig, c); err != nil {
				return err
			}
		}
		if models != nil {
			c.Models = *models
		}
		account, err = s.contributionAccount(c)
		if err != nil {
			return err
		}
	}
	return s.repo.Review(ctx, actor, c, approve, note, account)
}

func (s *ChannelContributionService) prepareContributionGroup(ctx context.Context, user int64, name, platform string, input *CreateGroupInput, c *ChannelContribution) error {
	input.Name, input.Platform = name, platform
	if input.RateMultiplier <= 0 || input.RPMLimit < 0 || len(input.Description) > 4096 || (input.SubscriptionType != "" && input.SubscriptionType != "standard" && input.SubscriptionType != "subscription") {
		return ErrContributionGroup
	}
	// 引用已有分组只允许使用贡献者当前可见的范围，审核时再次验证。
	groups, err := s.Groups(ctx, user)
	if err != nil {
		return err
	}
	allowed := map[int64]bool{}
	for _, g := range groups {
		allowed[g.ID] = true
	}
	refs := append([]int64{}, input.CopyAccountsFromGroupIDs...)
	if input.FallbackGroupID != nil {
		refs = append(refs, *input.FallbackGroupID)
	}
	if input.FallbackGroupIDOnInvalidRequest != nil {
		refs = append(refs, *input.FallbackGroupIDOnInvalidRequest)
	}
	for _, id := range refs {
		if !allowed[id] {
			return ErrContributionGroup
		}
	}
	if len(input.ModelRouting) > 0 {
		ids := make([]int64, 0, len(groups))
		for _, group := range groups {
			ids = append(ids, group.ID)
		}
		accounts, err := s.keys.groupRepo.GetAccountIDsByGroupIDs(ctx, ids)
		if err != nil {
			return err
		}
		allowedAccounts := make(map[int64]bool, len(accounts))
		for _, id := range accounts {
			allowedAccounts[id] = true
		}
		for _, targets := range input.ModelRouting {
			for _, id := range targets {
				if !allowedAccounts[id] {
					return ErrContributionGroup
				}
			}
		}
	}
	if s.groupPreparer == nil {
		return infraerrors.InternalServer("CONTRIBUTION_GROUP_UNAVAILABLE", "分组配置暂时不可用")
	}
	group, accounts, err := s.groupPreparer.PrepareGroup(ctx, input)
	if err != nil {
		return infraerrors.BadRequest("CONTRIBUTION_GROUP_CONFIG_INVALID", err.Error())
	}
	if c != nil {
		c.PreparedGroup, c.CopyAccountIDs = group, accounts
	}
	return nil
}

func (s *ChannelContributionService) contributionAccount(c *ChannelContribution) (*Account, error) {
	if err := validateContributionSettings(&c.ContributionSettings); err != nil {
		return nil, err
	}
	key, err := s.encryptor.Decrypt(c.KeyEncrypted)
	if err != nil {
		return nil, infraerrors.InternalServer("CONTRIBUTION_DECRYPT_FAILED", "无法读取申请 Key")
	}
	credentials := map[string]any{"base_url": c.BaseURL, "api_key": key}
	if len(c.Models) > 0 {
		mapping := map[string]any{}
		for _, model := range c.Models {
			mapping[model] = model
		}
		credentials["model_mapping"] = mapping
	}
	extra := map[string]any{"contribution_id": c.ID, "base_rpm": c.RPM, "quota_daily_limit": c.DailyLimit, "quota_limit": c.TotalLimit}
	if c.DailyLimit > 0 {
		tz := s.timezone
		if tz == "" {
			tz = "UTC"
		}
		extra["quota_daily_reset_mode"], extra["quota_daily_reset_hour"], extra["quota_reset_timezone"] = "fixed", 0, tz
	}
	return buildAccountForCreate(&CreateAccountInput{Name: c.SiteName, Platform: c.Platform, Type: AccountTypeAPIKey, Concurrency: c.Concurrency, Priority: 50, Credentials: credentials}, extra)
}

func (s *ChannelContributionService) Models(ctx context.Context, actor, id int64) ([]string, error) {
	c, err := s.repo.GetForReview(ctx, actor, id)
	if err != nil {
		return nil, err
	}
	if c.Status != "pending" {
		return nil, ErrContributionReviewed
	}
	account, err := s.contributionAccount(c)
	if err != nil {
		return nil, err
	}
	if s.modelFetcher == nil {
		return nil, infraerrors.InternalServer("CONTRIBUTION_MODELS_UNAVAILABLE", "模型获取暂时不可用")
	}
	models, err := s.modelFetcher.FetchUpstreamSupportedModels(ctx, account)
	if err != nil {
		return nil, infraerrors.BadRequest("CONTRIBUTION_MODELS_FAILED", "获取上游模型失败，请检查渠道地址和 Key")
	}
	return models, nil
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
