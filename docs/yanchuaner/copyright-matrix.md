# 版权与来源矩阵

## 审计方法

New API 上游基线固定为 `v1.0.0-rc.21`（`bde9b2f44887d34ec54799ae191d50f97914359e`）。所有受 Git 跟踪文件按以下互斥规则审计：

1. 基线已有且内容未变：保留为依赖。
2. 基线已有且内容已变，或新文件与上游界面/控制器紧密耦合：经开源许可证授权修改。
3. 有独立需求、设计、数据模型和测试的新文件：已自主实现。
4. 当前仍由上游承担、且阶段 C 需要替换的业务能力：计划替换。

运行 `scripts/audit-source-origin.ps1` 可列出每个跟踪文件相对基线的机械分类；人工矩阵用于区分“新增文件”和“独立原创模块”，不能通过改名或格式化改变来源。

## 保留为依赖

| 范围 | 来源 | 许可证/义务 |
| --- | --- | --- |
| 基线中未修改的 Go、React、路由、模型、协议适配代码 | QuantumNous/New API `v1.0.0-rc.21` | AGPLv3；保留 `LICENSE`、`NOTICE`、版权头、界面署名和上游链接 |
| `go.mod` / `go.sum` 直接与间接依赖 | 各上游模块 | 以 `THIRD-PARTY-LICENSES.md` 和 `NOTICE` 为准 |
| `web/bun.lock` 与前端包 | npm/Bun 包上游 | 以锁文件版本和各包许可证为准；本轮未新增依赖 |
| LiteLLM 镜像 | BerriAI/LiteLLM revision `b3086ccd74553565c9a39716e72303ae985555f9` | 非 `enterprise/` 内容为 MIT；镜像是否包含额外商业组件必须随升级复核 |
| Open WebUI 镜像 | Open WebUI `0.10.2`, revision `ecd48e2f718220a6400ecf49eafd4867a38feb10` | Open WebUI License；品牌变更受 50 用户/书面许可/企业许可条件限制 |
| PostgreSQL 镜像 | `16.14-alpine` | PostgreSQL License；Docker 官方镜像包装为 MIT |
| Redis 镜像 | `7.4.9-alpine` | RSALv2 或 SSPLv1；仅作内部缓存，不向用户提供 Redis 功能 |

## 经授权修改

以下修改依据 New API 的 AGPLv3 授权进行，不是燕中对上游代码版权的取得：

- `controller/oauth.go`、`controller/user.go`、`controller/token.go`、`controller/audit.go`、`controller/misc.go`
- `middleware/auth.go`
- `model/main.go`、`model/user.go`、`model/token.go`
- `service/funding_source.go`、`service/billing_session.go`
- `router/api-router.go`
- `Dockerfile`、`.dockerignore`
- `oauth/*.go` 中已登记的日志脱敏修补
- `web/default/src/features/keys/**`、`web/default/src/features/auth/types.ts`、身份/首页/侧栏/仪表盘等基于上游组件的界面改动
- `web/default/src/i18n/config.ts`、`languages.ts` 和中英文词条

这些文件必须继续保留原版权头。提交历史与 `PATCHES.md` 记录改动，不得标注为燕中独立原创文件。

## 计划替换

| 模块 | 优先级 | 阶段 C 目标 |
| --- | --- | --- |
| 用户余额、Token 与用量日志的业务真值 | P0/P1 | 独立权益账户、不可变流水、哈希虚拟 Key 和审计服务 |
| 管理员发放、活动和兑换码 | P1 | 自主活动模型、目标人群、领取次数、有效期和幂等兑换 |
| 供应商/模型权益与每 Key 限流 | P1 | provider/model allowlist、RPM、TPM、并发和来源策略 |
| BYOK | P1 | 独立 Vault/Broker、信封加密、所有者绑定、脱敏管理和零明文日志 |
| New API 用户端 | P1 | 自主控制台与 API，不继续扩展上游用户页面 |
| New API 管理端/网关耦合 | P2 | 自主控制面通过协议调用可替换网关，完成双写和切流 |
| Open WebUI 客户端层 | P2 | 自主燕中 AI 前端与核心交互，Open WebUI 降为可替换依赖 |

## 已自主实现

| 文件/目录 | 独立证据 | 当前许可 |
| --- | --- | --- |
| `model/yanchuaner_virtual_key.go` 及测试 | 独立密钥格式、哈希解析和防哈希重放测试 | 随仓库 AGPLv3-or-later |
| `model/yanchuaner_quota_ledger.go` 及测试 | 独立数据模型、幂等事务、不透支和初始赠额测试 | 随仓库 AGPLv3-or-later |
| `controller/yanchuaner_quota_ledger.go` | 独立流水查询与管理员发放适配 | 随仓库 AGPLv3-or-later |
| `service/yanchuaner_wallet_ledger_test.go` | 独立预扣、结算、退款行为证据 | 随仓库 AGPLv3-or-later |
| `model/yanchuaner_subject_grant.go`、`controller/yanchuaner_subject_grant.go` 及测试 | 独立主体、应用、受众、scope、短期签名、撤销和防跨应用重放协议 | 随仓库 AGPLv3-or-later |
| `controller/yanchuaner_subject_exchange.go` 及测试 | 独立主站令牌复验、服务客户端鉴权、可信 OAuth 绑定与固定 AI Web 策略 | 随仓库 AGPLv3-or-later |
| `model/yanchuaner_ai_session_key.go` 及测试 | 独立应用会话 Key 生命周期、并发会话指针、模型/预算边界、哈希存储与轮换策略 | 随仓库 AGPLv3-or-later |
| `docs/yanchuaner/**` | 燕中需求、架构、验收、迁移和来源设计 | 作者版权；仓库分发条件待运营主体/贡献政策确认 |
| `deploy/**`、`scripts/*integrated*`、`scripts/generate-deploy-env.ps1` | 燕中部署边界与密钥生成流程 | 作者版权；不得包含生产秘密 |

## 更新规则

每次升级或替换必须同时更新：上游 commit/digest、文件来源分类、许可证义务、数据迁移、测试证据和回滚镜像。未能定位源码 revision 或许可证的镜像不得进入生产。
