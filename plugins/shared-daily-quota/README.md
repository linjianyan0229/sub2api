# 全站每日共享额度插件

普通用户共同使用一个每日额度池。管理员（`role=admin`）沿用原计费策略。

## 安装

1. 先部署包含分组共享上限支持的 Sub2API 0.2.9 宿主。0.1.4 需要同步更新宿主，不能只升级插件包；即使旧宿主也显示 0.2.9，仍须包含本次代码。后端启动自动执行新增迁移 `246_shared_quota_group_limits.sql`。原版 0.2.8 不包含此扩展，不能单独安装本插件使用；多实例必须全部升级，共用数据库、加密密钥和站点时区。
2. 将构建产物 `trusted-publisher.yaml` 中的公钥条目合并到宿主配置的 `plugins.trusted_publishers`，保留现有发布者配置，重启宿主。
3. 管理后台 → 插件 → 上传 `shared-daily-quota-0.1.4.s2plugin`。
4. 打开插件配置，设置「全站每日共享额度（USD）」「全站每日共享额度刷新时间」和「额度生效方式」，可选择分组并填写「分组每日使用上限」，保存后启用。刷新时间使用宿主 `timezone`，通常为 `Asia/Shanghai`。

安装后默认停用，保存完整有效配置后才能启用。支持 Linux amd64、Linux arm64、Windows amd64。可与 OpenAI OAuth 传输插件共存。

0.1.1 修复配置页在宿主沙箱中点击保存无效的问题。已安装 0.1.0 时，先停用插件，再直接上传新包更新，已有配置保留；关闭旧配置窗口后重新打开。本次修复无需更新宿主。

0.1.2 修复公网 HTTP 访问时 `crypto.randomUUID is not a function` 导致配置无法加载、保存的问题。请求标识改用 HTTP 和沙箱均支持的 `crypto.getRandomValues`，Bridge 鉴权方式不变。升级保留已有配置，无需更新宿主镜像。

0.1.3 新增「立即生效 / 下次刷新时生效」。升级保留原配置，未选择时默认立即生效；旧版本的宿主不支持延后生效，必须先升级宿主，停用插件后上传新包，再按原状态启用。

0.1.4 新增分组每日上限：所有用户继续共用全站额度，所选分组增加每日使用上限，未选分组不受额外上限约束。升级保留已有配置，默认不添加分组上限。

## 额度与计费

- 所有普通用户、API Key、平台共享同一剩余额度，按原模型价格和倍率扣费；不修改原个人余额、不扣个人订阅、不累计个人 API Key 金额限额或用户平台额度。
- 分组上限按 API Key 在请求开始时绑定的分组累计，由同分组所有普通用户共享；它不是独立余额，每次请求只扣一次全站额度。全站额度耗尽会阻止所有普通用户新计费请求，单个分组上限耗尽只阻止该分组，均不会回落到个人计费。上限为 0 暂停该分组的新计费请求，移除配置表示取消额外上限。
- 分组与全站共用账期及刷新时间。立即生效同时更新全站额度和分组上限，保留已用与冻结；下次刷新生效同时延后新增、修改和移除分组上限。分组上限可大于全站额度，但实际仍受全站额度约束。
- 分组用量从部署 0.1.4 对应宿主后开始记录，升级前已完成请求不回溯分摊，原全站已用和冻结金额保持不变；升级后的未设上限分组也累计用量，随后增加上限不会重置其当期已用。升级前接受的异步任务仍按原快照完成共享结算。
- 用户面板、Key 查询和 `/v1/usage` 展示共享额度；个人充值、兑换、订阅及返佣转余额入口关闭。已有支付订单的回调、查询和退款仍可处理，历史个人数据保留。
- API Key 过期、禁用、IP 规则、用户/分组访问权限、并发和 RPM 限制仍然有效。上游账号自身成本限额继续执行。
- 额度为 0 表示暂不接受普通用户的新计费请求。每日到点进入新账期，剩余不结转；无请求时无需定时任务，下一次查询或请求会创建新账期。
- 沿用宿主后付费机制：准入时检查全站额度和分组上限的剩余金额，上游完成后原子扣除真实成本并按请求去重。并发在途请求可能使当日已用金额超出全站额度或分组上限，余额显示为 0，后续新请求返回 429。此插件不提供逐 token 硬停止或所有请求的成本预估冻结。
- 批量生图在提交时原子冻结共享额度，完成时扣实际金额并释放差额；失败/取消释放冻结。跨日、重启、停用后仍使用持久化的原账期结算。
- 普通流式/同步请求保存请求内计费快照；异步视频将创建时计费来源和模型快照持久化，异步图片继承请求上下文。已接受任务的查询、取消和结果读取不因共享额度耗尽而被余额检查拦截，仍需通过任务所有权校验。
- 「立即生效」在保存成功时更新当前总额，保留已用和冻结金额；降低至已用与冻结金额之和以下时，剩余显示为 0，不退还已消费额度。
- 「下次刷新时生效」保存目标额度，当前账期继续使用原总额，到下一次刷新时采用新额度并开始新账期。无访问时无需定时任务，下一次查询或请求会自动采用到点后的账期。重复修改以最后一次成功保存为准，切换为立即生效会取消原来的待生效额度。
- 修改刷新时间不会清空当前用量，当前账期结束时间改为新刷新时间的下一次到点。首次配置没有旧额度时，两种方式都以首次设置的额度开始。
- 配置、当前总额和下一次刷新时间在同一数据库事务中保存，分组上限也持久化在该共享账期；重启保留待生效配置，已读取的旧配置不能覆盖新总额。
- 停用后新请求恢复原个人策略，在途请求继续按原来源结算；停用/重新启用不会清空同一账期的用量。
- 数据库中已启用绑定是策略权威。插件进程退出不使普通用户回落到个人扣费；宿主继续按保存配置执行并恢复插件进程。策略读取或解密失败时拒绝计费请求。

## 配置契约

能力：`billing.shared_daily_quota.v1`，固定范围 `all/all`，不授予上游凭据权限，不能灰度启用。

```json
{"daily_quota":100,"reset_time":"00:00","apply_mode":"immediate","group_limits":{"1":20,"2":30}}
```

`daily_quota` 必填，0 至 10000000000，计费精度为小数点后 8 位；`reset_time` 为严格 `HH:mm`；`apply_mode` 可选 `immediate` 或 `next_reset`，省略时兼容旧配置，默认 `immediate`。`group_limits` 可选，键为真实分组 ID，值为 0 至 10000000000 的每日 USD 上限；省略或空对象表示不设额外分组上限。未知字段被拒绝。

## 构建

在 `backend` 目录执行（Go 版本以 `go.mod` 为准）：

```sh
go run ./cmd/package-shared-quota -output /absolute/output/directory -key /private/outside-repository/publisher.key
```

可通过 `-go` 指定 Go 可执行文件。首次构建生成 Ed25519 私钥，后续构建复用；私钥必须留在仓库外，分发仅包含 `.s2plugin` 与公钥配置。签名对应整个清单，清单包含每个运行时与 UI 文件的 SHA-256。

## 验证

```sh
go test ./pkg/pluginapi/sharedquota
go test -tags unit ./internal/service ./internal/repository ./internal/server/middleware -run TestSharedQuota
SUB2API_TEST_SHARED_QUOTA_PACKAGE=/absolute/shared-daily-quota-0.1.4.s2plugin go test ./internal/service -run TestSharedQuotaSignedPackageLifecycle -count=1
go test -tags integration ./internal/repository -run 'TestSharedQuotaRepository|TestSharedQuotaConfig|TestSharedQuotaGroup' -count=1
```

最后一项使用项目现有临时 PostgreSQL/Redis Docker 测试环境，不连接业务数据库。前端专项：在 `frontend` 执行 `pnpm exec vitest run src/__tests__/sharedQuotaPlugin.spec.ts src/views/user/__tests__/KeysView.spec.ts src/components/keys/__tests__/BulkEditKeysModal.spec.ts`。
