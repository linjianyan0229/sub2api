# 腾讯云镜像部署与更新

镜像仓库：`ccr.ccs.tencentyun.com/linjianyan/sub2api`。`latest` 跟随本项目发布，版本标签用于固定版本。应用镜像不包含 PostgreSQL、Redis 服务或业务数据。

## 使用外部 PostgreSQL、Redis

Linux 服务器安装 Docker Engine、Compose v2（支持 `up --wait`），把部署文件放在 `/opt/sub2api`：

```bash
sudo mkdir -p /opt/sub2api
sudo cp deploy/docker-compose.standalone.yml /opt/sub2api/docker-compose.yml
sudo cp deploy/.env.example /opt/sub2api/.env
sudo chmod 600 /opt/sub2api/.env
```

编辑 `.env`，保留已有值；新安装至少设置以下内容。密码、密钥由部署者填写，不要提交到 Git。数据库账号须有应用迁移所需的建表权限。

```dotenv
SUB2API_IMAGE=ccr.ccs.tencentyun.com/linjianyan/sub2api:latest
DATABASE_HOST=数据库地址
DATABASE_PORT=5432
DATABASE_USER=sub2api
DATABASE_PASSWORD=数据库密码
DATABASE_DBNAME=sub2api
DATABASE_SSLMODE=disable
REDIS_HOST=Redis地址
REDIS_PORT=6379
REDIS_USERNAME=
REDIS_PASSWORD=Redis密码
REDIS_DB=0
REDIS_ENABLE_TLS=false
JWT_SECRET=固定的随机密钥
TOTP_ENCRYPTION_KEY=固定的32字节十六进制密钥
ADMIN_EMAIL=管理员邮箱
ADMIN_PASSWORD=管理员初始密码
TZ=Asia/Shanghai
```

`DATABASE_SSLMODE` 和 `REDIS_ENABLE_TLS` 按外部服务要求配置。两项密钥可分别用 `openssl rand -hex 32` 生成；已有部署必须继续使用原值。容器的 `localhost` 指容器自身；连接宿主机使用可访问的宿主机地址或 `host.docker.internal`。外部服务须允许 Docker 网络访问。

```bash
sudo docker login ccr.ccs.tencentyun.com
cd /opt/sub2api
sudo docker compose pull sub2api
sudo docker compose up -d --no-deps --wait sub2api
```

已有部署只修改应用的 `image`，保留原 Compose 项目名、端口、挂载、配置和数据卷，不要覆盖现有 `.env`。`docker-compose.yml` / `docker-compose.local.yml` 仍可配套运行 PostgreSQL、Redis，外部服务使用 standalone 文件。

`/app/data` 必须持久化，它保存应用配置、插件等运行数据。现有共享额度插件继续保留；新实例需导入 `shared-daily-quota-0.1.4.s2plugin`，并将随包提供的 `trusted-publisher.yaml` 公钥配置合并到持久化的 `config.yaml`。镜像不携带本地预览数据或插件签名私钥。

## 自动更新

镜像部署通过拉取 CCR 的 `latest` 并重建应用容器更新，后台不再执行上游 GitHub 的二进制替换。更新检查由宿主机任务负责，后台版本菜单不声称已检查远端最新版本。

服务器需安装 `bash`、`python3` 和 `util-linux`（提供 `flock`）。先验证一次更新脚本，再启用 systemd 定时器（每次执行结束 5 分钟后再次检查）：

```bash
sudo cp deploy/update-ccr.sh /opt/sub2api/update-ccr.sh
cd /opt/sub2api
sudo bash ./update-ccr.sh -f docker-compose.yml --env-file .env
# 以下源文件路径按项目所在目录调整。
sudo cp /path/to/sub2api/deploy/sub2api-ccr-update.service /etc/systemd/system/
sudo cp /path/to/sub2api/deploy/sub2api-ccr-update.timer /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now sub2api-ccr-update.timer
sudo systemctl list-timers sub2api-ccr-update.timer
sudo journalctl -u sub2api-ccr-update.service
```

服务默认使用 root 的 Docker 登录凭据，所以服务器用 `sudo docker login` 登录。部署目录或 Compose 项目名不同，须先修改 service 的 `WorkingDirectory`、`ExecStart`（已有自定义项目名添加 `-p 原项目名`）。脚本也可由现有 cron/任务调度器调用；每次执行会使用现有 Compose 与 `.env`。

脚本只接受指定 CCR 仓库的 `latest`，只操作 `sub2api`。拉取失败不会重建；镜像没有变化时不会无故重启。启动/健康检查失败会报错并保留现场，不自动降级数据库。更新时应用有短暂中断，首次升级会执行数据库迁移（排行榜首次回填可能耗时），请预留窗口并保留数据库备份。

暂停更新：`sudo systemctl disable --now sub2api-ccr-update.timer`。固定版本时设置 `SUB2API_IMAGE=ccr.ccs.tencentyun.com/linjianyan/sub2api:版本号`，然后执行 Compose 的 pull/up；回退前确认数据库版本兼容。

## 构建与发布

在仓库根目录构建。先登录 CCR，再推送版本标签与 `latest`：

```bash
VERSION=0.3.49
IMAGE=ccr.ccs.tencentyun.com/linjianyan/sub2api
docker build --build-arg VERSION="$(cat backend/cmd/server/VERSION)" --build-arg RELEASE_VERSION="$VERSION" --label "org.opencontainers.image.version=$VERSION" -t "$IMAGE:$VERSION" -t "$IMAGE:latest" .
docker save -o sub2api-image.tar "$IMAGE:$VERSION" "$IMAGE:latest"
docker push "$IMAGE:$VERSION"
docker push "$IMAGE:latest"
```

网站发布版本与插件兼容版本分开：`RELEASE_VERSION` 注入网站公开设置、管理员版本接口和命令行版本显示；插件继续使用 `VERSION`（取自 `backend/cmd/server/VERSION`）检查兼容范围。未指定发布版本时，显示回退到 `VERSION`。CCR 工作流自动传入镜像版本作为 `RELEASE_VERSION`；直接编译二进制时使用 `-X main.ReleaseVersion=0.3.49`。离线主机可 `docker load -i sub2api-image.tar` 后启动。更新仍需要主机能够访问 CCR 并拥有拉取权限。

镜像 `0.3` 包含论坛和共享额度生效方式支持，应用兼容版本仍为 `0.2.9`，兼容现有共享额度插件的版本范围。共享额度的新配置页面由插件 `0.1.3` 提供，已有实例需按插件说明另行升级持久化插件包；更新镜像不会覆盖已安装插件。

镜像 `0.3.1` 将论坛全站通报改为榜单用户上线触发，并按登录会话去重；启动时执行 `245_forum_arrivals.sql`，升级前保留数据库备份。应用兼容版本继续为 `0.2.9`。

镜像 `0.3.2` 配合共享额度插件 `0.1.4` 新增分组每日使用上限，启动时执行 `246_shared_quota_group_limits.sql`。先备份数据库并更新宿主，再停用、上传新插件包并按原状态启用；升级过程应暂停外部计费请求，避免停用期间回落到个人计费。已有全站额度、当期用量和刷新时间保留，默认不添加分组上限。网站显示版本为 `0.3.2`，插件兼容版本仍为 `0.2.9`；仅升级插件包不足以启用新功能。

镜像 `0.3.3` 修复用户面板缺少分组额度用量的问题：在全站汇总下展示当前用户可用分组的有效每日上限、已用、冻结和剩余金额；下次刷新才生效的配置不会提前展示。继续使用插件 `0.1.4`，无需重装插件或新增数据库迁移。

镜像 `0.3.4` 将分组金额展示精度提高到最多八位小数，避免小额用量显示为零；插件与计费策略保持不变。

镜像 `0.3.41` 新增管理员全站共享额度展示和论坛管理员标识，并约束非空昵称唯一（忽略大小写、去除首尾空白）。启动时执行 `247_user_username_rules.sql`：普通用户的保留昵称“管理员”会自动改为“小丑”（重名时追加序号），管理员个人计费策略保持不变。升级前备份数据库，并检查未删除用户的历史昵称规范化后是否重名；存在冲突时迁移会回滚，须先处理冲突再升级。继续使用插件 `0.1.4`，无需重装。

镜像 `0.3.42` 新增渠道贡献申请与管理员审核，批准后创建渠道账号并绑定已有分组或新建分组，分组展示贡献者昵称。申请中的 API Key 加密保存，不在申请列表返回；审核结束后清除申请中的密钥副本。启动时执行 `248_channel_contributions.sql` 创建申请表和索引，升级前保留数据库备份。外部 PostgreSQL、Redis 配置及共享额度插件 `0.1.4` 沿用原值。

镜像 `0.3.43` 为贡献渠道新增 RPM、并发、每日及累计额度、模型选择和新分组配置，并提供渠道额度页面。启用共享额度时，配置了独立金额上限的已审核渠道按账号成本计费，不重复扣除全站或个人额度；全站额度耗尽后仍可调度有可用独立额度的渠道。未配置独立金额上限的渠道继续使用全站额度，管理员沿用原计费策略。启动时执行 `249_channel_contribution_limits.sql`，新增申请设置、RPM 计数与批量任务冻结记录；升级前备份数据库。插件仍为 `0.1.4`，外部 PostgreSQL、Redis 配置保持不变。

镜像 `0.3.44` 支持贡献者编辑自己的已审核渠道连接、密钥、容量、模型及状态页展示开关；密钥留空保留原值，既有用量、分组绑定及管理员运行状态保留。额度列表过滤不可用渠道后再分页，状态页按贡献者开关展示渠道状态。无新增数据库迁移，继续使用插件 `0.1.4` 及现有外部 PostgreSQL、Redis 配置。

镜像 `0.3.45` 将贡献渠道状态页接入真实监控，展示可用率、延迟时间线及模型详情；贡献者可设置检测模型和 OpenAI 检测协议。主动检测受全站监控模式、状态页开关及渠道可用状态约束，使用关联账号的当前连接和密钥，按模型映射请求上游；已有开启状态展示的渠道会在启动时同步监控配置。贡献渠道的列表与详情沿用分组可见权限，关闭展示后停止检测并保留历史。无新增数据库迁移，继续使用插件 `0.1.4` 及现有外部 PostgreSQL、Redis 配置。

镜像 `0.3.49` 包含此前 `0.3.46` 至 `0.3.48` 的论坛置顶更新：置顶消息通过事件接口同步到全站页头，支持轮播、展开完整内容和移动端紧凑展示；退出或切换账号时清空置顶缓存。管理员和用户用量记录新增输出速度（t/s），以输出 token 数除以扣除首 token 等待后的生成耗时计算，缺少首 token 耗时时使用总耗时，无效数据展示为 `-`。无新增数据库迁移，应用兼容版本仍为 `0.2.9`，继续保留现有共享额度插件及外部 PostgreSQL、Redis 配置。

代码仓库：`https://github.com/linjianyan0229/sub2api`。本地 `origin` 和默认推送分支指向自有仓库；`upstream` 仅用于读取原项目更新，已禁用推送。版本直接提交到自有仓库，无需向上游提交合并请求。保留原项目许可证和 Go 模块路径。

也可在自有 GitHub 仓库配置 `CCR_USERNAME`、`CCR_PASSWORD` Secrets，手动运行“发布腾讯云 CCR 镜像”工作流，输入新的版本号；它构建 amd64/arm64 并发布到同一镜像仓库。发布前需先将本地修改提交并推送到自有代码仓库。原上游 release 工作流保留手动入口，自有仓库推送版本标签不会自动执行它。
