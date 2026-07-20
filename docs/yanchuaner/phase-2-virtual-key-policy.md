# 阶段 2C：YanCore 虚拟 Key 策略

更新日期：2026-07-21
状态：`IMPLEMENTED_FLAG_OFF`

## 目标与边界

本模块为哈希虚拟 Key 增加燕中自主策略语义和修订审计，不复制 New API 的 Token 实现。当前兼容期继续由 `tokens` 承载预算、有效期、模型白名单和来源 IP，并由 YanCore 策略表承载供应商、RPM、TPM、并发、状态和版本。两部分共同构成一个虚拟 Key 的有效策略。

首期只允许 OpenAI 与 DeepSeek 的标准文本模型。BYOK、充值、异步任务限流和跨资金来源自动回退不在本阶段开放。

## 数据模型

| 表 | 作用 | 不变量 |
| --- | --- | --- |
| `yan_core_virtual_key_policies` | 每个 Token 一条当前策略 | `token_id` 唯一；活动策略不得使用通配供应商；版本单调递增 |
| `yan_core_virtual_key_policy_revisions` | 创建、回填和每次更新的不可变快照 | 保存操作者、原因、供应商、模型、来源、速率、状态和版本；业务代码不更新或删除旧记录 |
| `tokens` | B 阶段兼容投影 | Key 只保存哈希；有限预算、有效期、模型白名单和来源 IP 继续由成熟网关校验 |

默认限制为 `60 RPM / 100000 TPM / 2 并发`。API 中的零值表示创建时使用默认值、更新时保留现值，不表示无限制。

## API

- 创建哈希 Key：现有 `POST /api/token/` 可带 `yancore_policy`；策略与 Token、首条修订在同一事务提交。
- `GET /api/yancore/virtual-key-policies/:token_id`：读取本人 Key 的当前策略。
- `PUT /api/yancore/virtual-key-policies/:token_id`：更新供应商、RPM、TPM、并发或状态；`reason` 必填。
- `GET /api/yancore/virtual-key-policies/:token_id/revisions`：按版本倒序读取最近 100 条修订。

活动供应商集合只能由 `openai`、`deepseek` 组成，并且必须覆盖 Token 模型白名单推导出的供应商。`*` 只用于无法安全识别的历史 Key 禁用占位，不能启用。

## 请求执行顺序

1. 认证提交的 Key，哈希查找 Token；继续执行 Token 的状态、有效期、模型和来源 IP 检查。
2. 开关开启时读取 YanCore 策略；缺失或禁用返回 403，不静默放行。
3. 网关选定渠道后复核供应商。DeepSeek/OpenAI 的模型前缀优先于 OpenAI 兼容渠道类型，防止兼容协议掩盖实际供应商语义。
4. 标准文本请求在转发前原子预留 RPM、估算 TPM 和并发。TPM 估算为提示 Token 加请求声明的最大输出 Token。
5. 收到真实用量后按实际总 Token 调整 TPM；无有效用量或请求失败时退回 TPM 预留；并发槽始终释放。
6. 预算继续进入既有预扣、结算、退款和不可变额度流水；YanCore 限流不创建第二套余额。

Redis 已配置但不可用时限流拒绝请求并返回 503。未配置 Redis 时使用单进程内存限流，仅适合本地开发；多副本或公开预览必须使用 Redis。

## 迁移与启用

1. 保持 `YANCHUANER_VIRTUAL_KEY_POLICY_ENABLED=false` 启动新版本，完成表迁移。
2. 盘点所有 `key_hash_enabled=true` 的 Token，确认模型白名单只含首期模型；先轮换仍为明文的旧 Key。
3. 在隔离数据库开启开关执行回填。可推导供应商的 Key 创建活动策略；空白、通配或未知模型的 Key 创建为禁用策略并留下原因。
4. 审查禁用清单，逐个修正模型范围和策略，不批量扩大权限。
5. 验证 Redis、403/429/503、预算扣减、失败退款和修订查询后，再对本地集成栈开启开关。

## 验证证据

- SQLite：策略创建/更新事务、版本与修订、供应商推导、回填禁用、AI Web 会话策略、RPM/TPM/并发及 Redis 失效路径。
- PostgreSQL 16.14 与 MySQL 8.0：建表、Token/策略同事务创建、行锁更新和两条不可变修订。
- Redis 7：Lua 原子并发拒绝、TPM 实际结算和请求中途客户端失效时 fail-closed。
- 全量 Go 测试、仅编译检查、Compose 解析和 Docker 构建作为合并门禁。

## 回滚

发现策略误拒绝时，先冻结新调用并保留最后请求 ID，再回滚到固定镜像。紧急关闭策略开关会退回 Token 的模型、来源、预算和有效期约束，但会失去独立供应商与每 Key 速率限制，因此不能在 Agent 公网流量继续运行时把关开关作为长期回滚方案。策略表和修订流水不删除。

## 已知限制与下一步

- RPM/TPM/并发目前只覆盖标准同步文本 Relay；异步任务只执行供应商复核，尚未持久化限流预留。
- 模型、来源 IP、预算和有效期仍是 New API Token 兼容投影；通过旧 Token 更新接口修改时不会生成新的 YanCore 策略修订，阶段 C 控制面需把这些字段移入自主事务。
- 固定分钟窗口适合预览期，不能代替长期的分布式配额、成本告警和异常用量风控。
- 策略 API 目前是后端契约；极简用户端和管理员批量策略操作在下一 UI slice 实现。
