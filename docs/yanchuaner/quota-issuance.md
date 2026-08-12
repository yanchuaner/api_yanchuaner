# 公益额度发放与调整

燕中生态不接在线支付，避免资金与合规风险。额度只通过线下收款（微信转账或面对面）后，由管理员在控制面发放。

## 发放接口

`POST /api/yanchuaner/admin/quota`，仅 root 可调用。

请求体：

```json
{
  "user_id": 1,
  "action": "grant",
  "amount": 5000,
  "reason": "微信收款 50 元，公益额度",
  "reference": "wx-20260812-0001"
}
```

- `action`：`grant`（只允许正数）或 `adjust`（正负均可，用于回退）。
- `reason`：必填，最长 200 字。
- `reference`：线下收款凭证号（建议用微信转账单号或日期+金额），必填；也可直接传 `idempotency_key`。

接口以 `reference` 或 `idempotency_key` 作为幂等键：同一笔收款重复提交只发放一次；换金额复用同一凭证会被拒绝（409）。

写入行为：

- 事务内锁定用户行，追加不可变额度流水（`public_benefit`），同步更新兼容余额投影。
- 透支、超范围和参数错误直接拒绝，不会产生部分写入。
- 每次发放/调整都写入管理员审计日志，包含金额、原因、凭证号和最新余额。

返回：

```json
{
  "success": true,
  "data": {
    "entry_id": 123,
    "balance_after": 6000,
    "idempotency_key": "quota:ref:wx-20260812-0001"
  }
}
```

管理员也可以通过自主 ai-web 的“额度发放”面板操作同一接口；ai-web 不保存 root 凭据，而是用携带管理员声明的 YanCore grant 调用 `POST /api/yancore/admin/quota`。API 会再次核对数据库中的 root 角色与启用状态，grant 过期或撤销后立即失效。

## 用户侧核对

用户登录 ai-web 后可见公益额度；每次对话显示 request ID 和输入/输出 token 用量，额度随结算实时更新。用户也可通过 `GET /api/yanchuaner/quota-ledger` 查看自己的不可变流水。

## 对账

- `GET /api/yanchuaner/requests/:request_id`：登录用户查询自己某个 request ID 的全部流水（预扣、结算、退款）。
- `GET /api/yanchuaner/admin/requests/:request_id`：root 查询任意用户的同一 request ID 流水，用于与 DeepSeek 账单核对。

每次模型请求都应看到“预扣 + 结算/退款”成对出现；只出现预扣没有结算或退款的请求即为异常，应优先排查。

退款路径由 `service/yanchuaner_wallet_ledger_test.go`、`service/task_billing_test.go` 与 `service/yanchuaner_campaign_funding_test.go` 覆盖：重复退款幂等、任务失败退款、超收差额退款和活动额度退款均不重复入账。故障注入验收在本地隔离库运行，不在生产主站执行。

## 预算与告警

- `GET /api/yanchuaner/admin/budget`：root 查询当日与当月的公益净消费、发放额度、消费人数和当前预算阈值。
- 通过 `YANCHUANER_DAILY_BUDGET_UNITS` 与 `YANCHUANER_MONTHLY_BUDGET_UNITS` 配置阈值；返回的 `over_daily` / `over_monthly` 供监控或告警脚本使用。
