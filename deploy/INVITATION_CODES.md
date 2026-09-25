# 邀请码注册

在后台「兑换码」生成邀请码时设置人数上限（1～2147483647）。每行的「配置与使用记录」可修改上限、查看用户、注册 IP 和使用时间。上限不能小于已使用人数；满额后调高可继续使用，过期或禁用的邀请码仍不可用。

启用邀请码注册后，邮箱及 OAuth 新用户均占用邀请码名额。同一 IP 在所有邀请码之间只允许成功注册一个账号；IPv4 映射 IPv6 地址会归一化为同一个 IP。已有账号登录及管理员手工创建账号不受该注册限制影响。

用户创建、邀请码名额和 IP 占用通过数据库事务与唯一约束保证并发一致性。注册失败回滚不会消耗名额；正常删除用户或邀请码不会释放已成功注册的 IP。使用记录接口仅管理员可访问。

迁移 `243_reusable_invitation_codes.sql` 为旧邀请码保留 1 人上限，并回填现存的使用用户和时间。旧数据未保存注册 IP，显示「未知」，无法用于追溯限制旧账号的 IP。

注册 IP 使用 `server.trusted_proxies` 配置的可信代理链。反向代理部署必须配置实际代理地址并正确传递客户端 IP，不能通过开启原始转发头信任来替代此配置。共用公网 IP 的网络只能注册一个账号，这是按 IP 限制的既定行为。

验证命令：

```sh
cd backend
go test -tags unit ./internal/service ./internal/handler/... ./internal/server/middleware -run 'Invitation|OAuthEmail|EmailOAuth|Redeem|SessionBindingContext' -count=1
go test -tags integration ./internal/repository -run 'TestInvitation|TestRedeemCodeRepoSuite' -count=1
cd ../frontend
pnpm exec vitest run src/components/admin/__tests__/InvitationUsageDialog.spec.ts
pnpm run build
```
