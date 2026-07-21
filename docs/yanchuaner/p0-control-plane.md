# P0 自主控制面设计：哈希虚拟 Key 与公益额度流水

## 范围

P0 只覆盖主站认证成员使用 OpenAI/DeepSeek 标准文本模型的链路：登录、初始公益额度、虚拟 Key、调用、扣减和审计。充值、订阅、签到、异步任务、活动兑换和 BYOK 不在本模块中启用。

## 数据模型

### `tokens` 兼容扩展

- `key_hash_enabled`：区分旧明文 Token 和燕中哈希虚拟 Key。
- `key`：哈希 Key 保存 `sha256:<64 hex>`，不是 bearer credential。
- `key_display_prefix` / `key_display_suffix`：只用于脱敏展示。
- 现有名称、模型范围、预算、有效期、IP 来源和 group 字段继续作为过渡兼容投影。

### `quota_ledger_entries`

| 字段 | 约束 |
| --- | --- |
| `idempotency_key` | 全局唯一；同键同语义重试为 no-op，不同语义拒绝 |
| `amount` | 正数入账、负数扣减；零值拒绝 |
| `balance_after` | 同一数据库事务中的余额投影结果，不得小于 0 或超过 int32 |
| `entry_type` | `opening_balance`、`grant`、`reserve`、`settlement`、`refund`、`adjustment` |
| `funding_source` | P0 仅 `public_benefit`；旧余额迁移标记 `legacy` |
| `request_id` / `token_id` | 与调用日志和虚拟 Key 关联 |
| `actor_user_id` / `reason` | 管理员发放与人工调整追责 |

模型层不提供更新或删除接口。数据库级防篡改触发器因 SQLite/MySQL/PostgreSQL 差异留到独立控制面阶段，生产数据库权限必须先禁止应用账号执行手工 UPDATE/DELETE。

## API 契约

- `POST /api/token/`：启用 `YANCHUANER_HASHED_KEYS_ENABLED` 时只在本次响应的 `data.key` 返回完整 Key。
- `POST /api/token/:id/key`：哈希虚拟 Key 固定返回 HTTP 410，不得取回明文。
- `POST /api/token/batch/keys`：跳过哈希虚拟 Key。
- `GET /api/yanchuaner/quota-ledger`：仅返回当前登录用户的分页流水。

旧 Token 接口为迁移兼容面，不是阶段 C 的最终 API。

## 事务与幂等

1. OAuth 用户创建、OAuth 绑定和初始 `grant` 在同一事务中完成。
2. 余额变更先锁定用户行，计算非负余额，再更新 `users.quota` 并追加流水。
3. 标准请求使用 `request:<request_id>:wallet:<phase>` 作为幂等键。
4. 用量日志和流水共享 `request_id`，但当前仍分两次写入；日志库故障不回滚已完成的资金事务。阶段 C 应使用 outbox 消除该缺口。

## 迁移

启动迁移自动增加字段和流水表。启用 `YANCHUANER_QUOTA_LEDGER_ENABLED` 后，对尚无流水的已有用户追加一条不改变余额的 `opening_balance`。已有明文 Token 不自动转换；必须新建哈希 Key、验证调用、撤销旧 Key。

## 回滚

1. 保留数据库字段与流水表，不执行降级 DROP。
2. 将 `YANCHUANER_QUOTA_LEDGER_ENABLED=false` 可临时恢复旧钱包更新路径；已有流水冻结保留。
3. 将 `YANCHUANER_HASHED_KEYS_ENABLED=false` 只停止创建新哈希 Key；已创建哈希 Key 仍可认证和撤销。
4. 回滚镜像前备份 PostgreSQL，并记录最后一个流水 ID 与最后一个 New API 日志 request ID。
5. 恢复后核对余额投影、流水合计和供应商账单；不一致时冻结调用，不做静默修正。

## 验收证据

- OAuth 风格用户创建产生 `$1` 公益 `grant` 流水。
- 数据库不含新虚拟 Key 明文，提交数据库哈希无法认证。
- 重放同一幂等请求不重复扣费，语义冲突被拒绝。
- 余额不足时流水和余额均不变化。
- 预扣、差额结算和失败退款的金额与最终余额一致。
- 前端类型检查、定向 lint、Go 定向测试、PostgreSQL 迁移、Compose 解析和真实 OpenAI/DeepSeek 最小调用全部通过。
