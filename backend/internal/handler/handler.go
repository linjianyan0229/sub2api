package handler

import (
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/handler/admin"
	"github.com/Wei-Shaw/sub2api/internal/securityaudit"
)

// AdminHandlers contains all admin-related HTTP handlers
type AdminHandlers struct {
	Dashboard              *admin.DashboardHandler
	User                   *admin.UserHandler
	Group                  *admin.GroupHandler
	Account                *admin.AccountHandler
	Announcement           *admin.AnnouncementHandler
	DataManagement         *admin.DataManagementHandler
	Backup                 *admin.BackupHandler
	OAuth                  *admin.OAuthHandler
	OpenAIOAuth            *admin.OpenAIOAuthHandler
	GeminiOAuth            *admin.GeminiOAuthHandler
	AntigravityOAuth       *admin.AntigravityOAuthHandler
	GrokOAuth              *admin.GrokOAuthHandler
	CNProvider             *admin.CNProviderHandler
	Proxy                  *admin.ProxyHandler
	Redeem                 *admin.RedeemHandler
	Promo                  *admin.PromoHandler
	Setting                *admin.SettingHandler
	Ops                    *admin.OpsHandler
	System                 *admin.SystemHandler
	Subscription           *admin.SubscriptionHandler
	Usage                  *admin.UsageHandler
	UserAttribute          *admin.UserAttributeHandler
	ErrorPassthrough       *admin.ErrorPassthroughHandler
	TLSFingerprintProfile  *admin.TLSFingerprintProfileHandler
	Plugin                 *admin.PluginHandler
	APIKey                 *admin.AdminAPIKeyHandler
	ScheduledTest          *admin.ScheduledTestHandler
	Channel                *admin.ChannelHandler
	ChannelMonitor         *admin.ChannelMonitorHandler
	ChannelMonitorTemplate *admin.ChannelMonitorRequestTemplateHandler
	ContentModeration      *admin.ContentModerationHandler
	PromptAudit            *securityaudit.PromptAdminHandler
	Payment                *admin.PaymentHandler
	Affiliate              *admin.AffiliateHandler
	Compliance             *admin.ComplianceHandler
	AuditLog               *admin.AuditLogHandler
}

// Handlers contains all HTTP handlers
type Handlers struct {
	Contributions *ChannelContributionHandler

	Forum            *ForumHandler
	Auth             *AuthHandler
	User             *UserHandler
	APIKey           *APIKeyHandler
	Usage            *UsageHandler
	Redeem           *RedeemHandler
	Subscription     *SubscriptionHandler
	Announcement     *AnnouncementHandler
	ChannelMonitor   *ChannelMonitorUserHandler
	ChannelMonitorV2 *ChannelMonitorV2Handler
	Admin            *AdminHandlers
	Gateway          *GatewayHandler
	OpenAIGateway    *OpenAIGatewayHandler
	Setting          *SettingHandler
	Totp             *TotpHandler
	Passkey          *PasskeyHandler
	Payment          *PaymentHandler
	PaymentWebhook   *PaymentWebhookHandler
	AvailableChannel *AvailableChannelHandler
	ModelPlaza       *ModelPlazaHandler
	AsyncImage       *AsyncImageHandler
	BatchImage       *BatchImageHandler
}

// BuildInfo contains build-time information
type BuildInfo struct {
	Version        string // 插件兼容性使用的宿主版本。
	ReleaseVersion string // 定制发布版本，仅用于网站和更新信息。
	BuildType      string // source、release 或 container
}

func (b BuildInfo) DisplayVersion() string {
	if version := strings.TrimSpace(b.ReleaseVersion); version != "" {
		return version
	}
	return b.Version
}
