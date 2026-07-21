# 阶段 2C/2D/2E：YanCore 虚拟 Key 策略、控制台与安全回填

更新日期：2026-07-21
状态：`IMPLEMENTED_LOCAL_ACCEPTANCE_PENDING`

## 目标与边界

本模块为哈希虚拟 Key 增加燕中自主策略语义和修订审计，不复制 New API 的 Token 实现。当前兼容期继续由 `tokens` 承载预算、有效期、模型白名单和来源 IP，并由 YanCore 策略表承载供应商、RPM、TPM、并发、状态和版本。两部分共同构成一个虚拟 Key 的有效策略。

首期只允许 OpenAI 与 DeepSeek 的标准文本模型。BYOK、充值、异步任务限流和跨资金来源自动回退不在本阶段开放。

## 数据模型

| 表 | 作用 | 不变量 |
| --- | --- | --- |
| `yan_core_virtual_key_policies` | 每个 Token 一条当前策略 | `token_id` 唯一；活动策略不得使用通配供应商；版本单调递增 |
| `yan_core_virtual_key_policy_revisions` | 创建、回填和每次更新的不可变快照 | 保存操作者、原因、供应商、模型、来源、有限预算、有效期、Token/策略状态、速率和版本；业务代码不更新或删除旧记录 |
| `tokens` | B 阶段兼容投影 | Key 只保存哈希；有限预算、有效期、模型白名单和来源 IP 继续由成熟网关校验 |

默认限制为 `60 RPM / 100000 TPM / 2 并发`。API 中的零值表示创建时使用默认值、更新时保留现值，不表示无限制。

## API

- 创建哈希 Key：现有 `POST /api/token/` 可带 `yancore_policy`；策略与 Token、首条修订在同一事务提交。
- `GET /api/yancore/virtual-key-policies/:token_id`：读取本人 Key 的当前策略。
- `PUT /api/yancore/virtual-key-policies/:token_id`：在一个数据库事务中更新供应商、RPM、TPM、并发、状态，以及 Token 兼容投影中的名称、模型、来源 IP、有限预算、有效期与分组；`reason` 必填。
- `GET /api/yancore/virtual-key-policies/:token_id/revisions`：按版本倒序读取最近 100 条修订。
- `GET /api/yancore/virtual-key-policies/rollout/`：仅管理员读取历史哈希 Key 的脱敏预检报告。报告不含原始或展示 Key，只列出 Token ID、用户 ID、模型范围、分类和原因。
- `POST /api/yancore/virtual-key-policies/rollout/`：仅管理员以最多 100 个明确 `token_ids` 和 3 至 160 字符的 `reason` 执行一次回填批次。安全可推导项创建活动策略；其余项创建禁用策略，并冻结仍启用的不安全旧 Key。每项均追加不可变修订，管理员操作写入管理审计。

活动供应商集合只能由 `openai`、`deepseek` 组成，并且必须覆盖 Token 模型白名单推导出的供应商。`*` 只用于无法安全识别的历史 Key 禁用占位，不能启用。

策略开关开启后，受管理哈希 Key 不允许再通过旧 `PUT /api/token/` 修改或启停，旧接口固定返回 HTTP 409。用户控制台创建时提交速率策略，编辑时同时读取 Token 和策略，保存时只调用上述原子接口；快速启停同步修改 Token 状态与 YanCore 策略状态。

## 请求执行顺序

1. 认证提交的 Key，哈希查找 Token；继续执行 Token 的状态、有效期、模型和来源 IP 检查。
2. 开关开启时读取 YanCore 策略；缺失或禁用返回 403，不静默放行。
3. 网关选定渠道后复核供应商。DeepSeek/OpenAI 的模型前缀优先于 OpenAI 兼容渠道类型，防止兼容协议掩盖实际供应商语义。
4. 标准文本请求在转发前原子预留 RPM、估算 TPM 和并发。TPM 估算为提示 Token 加请求声明的最大输出 Token。
5. 收到真实用量后按实际总 Token 调整 TPM；无有效用量或请求失败时退回 TPM 预留；并发槽始终释放。
6. 预算继续进入既有预扣、结算、退款和不可变额度流水；YanCore 限流不创建第二套余额。

只有 OpenAI Chat/Completions 与 Responses 文本协议进入当前策略执行面。Image、Audio、Realtime、Embedding、异步 Task 和 Midjourney 对受策略管理 Key 返回 403；在持久化预留、真实用量结算和失败恢复完成前不得开放这些路径。

Redis 已配置但不可用时限流拒绝请求并返回 503。未配置 Redis 时使用单进程内存限流，仅适合本地开发；多副本或公开预览必须使用 Redis。

## 迁移与启用

1. 保持 `YANCHUANER_VIRTUAL_KEY_POLICY_ENABLED=false` 启动新版本，完成表迁移。启动过程不会自动写入历史回填。
2. 轮换仍为明文的旧 Key；随后由管理员调用 `GET /api/yancore/virtual-key-policies/rollout/` 盘点哈希 Key，确认模型白名单、有限预算、有效期和来源范围。
3. 审查预检结果，以小批量明确 Token ID 调用 `POST /api/yancore/virtual-key-policies/rollout/`。空白、通配、未知模型、无有限预算、过期或非启用 Key 只能生成禁用策略，不能批量扩大权限。
4. 查询管理审计和策略修订；逐个修正禁用项的模型/预算/状态，再通过 YanCore 原子接口启用。
5. 验证 Redis、403/429/503、预算扣减、失败退款和修订查询后，再对本地集成栈开启开关。
6. 使用本地 root 访问令牌运行 `scripts/verify-virtual-key-policy.ps1`；脚本必须验证版本从 1 增至 2、修订恰为两条且旧 Token 更新返回 409。

## 验证证据

- SQLite：策略创建/更新事务、版本与修订、供应商推导、显式回填预检/活动或禁用结果、AI Web 会话策略、RPM/TPM/并发及 Redis 失效路径。
- PostgreSQL 16.14 与 MySQL 8.0：建表、Token/策略同事务创建、行锁更新和两条不可变修订。
- Redis 7：Lua 原子并发拒绝、TPM 实际结算和请求中途客户端失效时 fail-closed。
- 全量 Go 测试、仅编译检查、Compose 解析和 Docker 构建作为合并门禁。

## 回滚

发现策略误拒绝时，先冻结新调用并保留最后请求 ID，再回滚到固定镜像。紧急关闭策略开关会退回 Token 的模型、来源、预算和有效期约束，但会失去独立供应商与每 Key 速率限制，因此不能在 Agent 公网流量继续运行时把关开关作为长期回滚方案。策略表和修订流水不删除。

## 已知限制与下一步

- RPM/TPM/并发目前只开放标准同步文本 Relay；异步和媒体协议已明确拒绝，尚未实现持久化限流预留。
- 模型、来源 IP、预算和有效期仍存放在 New API Token 兼容列，但已只能通过 YanCore 事务更新并追加修订；阶段 C 仍需把存储真值迁出 Token。
- 固定分钟窗口适合预览期，不能代替长期的分布式配额、成本告警和异常用量风控。
- 极简用户端已接入创建、查看、编辑和启停；管理员已具备 API 级预检与小批量回填能力，运营 UI 仍在下一 slice 实现。
