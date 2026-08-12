# YanCore API 契约

本文是燕中 API 面向自主 ai-web、YCZX Code 与后续 Agent 的稳定接入契约。实现细节见对应阶段文档，本文只维护端点、鉴权和调用约定。

## 鉴权模型

| 场景 | 凭据 | 说明 |
| --- | --- | --- |
| 用户模型调用 | `Authorization: Bearer sk-yc_...` | 个人虚拟 Key，只校验哈希，不再返回明文 |
| AI Web BFF 交换 | Basic `ai-yancore-bff` 客户端 | 用主站短期访问令牌换取 YanCore grant |
| AI Web 会话 | `Authorization: Bearer <grant>` | grant 最长 15 分钟，过期或撤销立即失效 |
| 主站身份事件 | HMAC 签名 + 事件时间 + `event_id` | 服务端到服务端 |
| 管理员操作 | root 会话，或带 `adm` 声明的 grant | grant 路径仍会复查数据库 root 角色 |

## 端点

| 方法 | 路径 | 鉴权 | 用途 |
| --- | --- | --- | --- |
| POST | `/api/yancore/subject-exchange` | BFF Basic | 主站令牌换 grant 与一次性会话 Key |
| POST | `/api/yancore/grants/introspect` | grant | 校验 grant 并返回主体、余额 |
| POST | `/api/yancore/identity-events` | HMAC | 账号停用、角色变化、强制退出 |
| GET | `/api/yancore/me/ledger` | grant | 个人不可变额度流水 |
| GET | `/api/yancore/me/keys` | grant | 个人虚拟 Key 列表（只含掩码） |
| POST | `/api/yancore/me/keys` | grant | 创建个人虚拟 Key，明文只返回一次 |
| DELETE | `/api/yancore/me/keys/:id` | grant | 删除自己的 Key |
| POST | `/api/yancore/admin/quota` | 管理员 grant | 线下收款后发放或调整公益额度 |
| GET | `/api/yanchuaner/admin/budget` | root | 日/月消费、发放与超预算标记 |
| GET | `/api/yanchuaner/admin/requests/:request_id` | root | 按 request ID 对账 |
| GET | `/api/yanchuaner/requests/:request_id` | 用户会话 | 用户核对自己的 request ID 流水 |
| POST | `/v1/chat/completions` | 个人 Key | DeepSeek 模型数据面 |

## 错误约定

- YanCore grant 无效或已撤销：401。
- 非管理员调用管理接口：403。
- 幂等键被不同参数复用：409。
- 额度透支、参数越界：400。
- 依赖服务不可用：502/503。

响应统一为 JSON：成功 `{"success":true,"data":...}`，失败 `{"success":false,"message":"..."}`。HTTP 状态码是判定依据，不能只看 `success`。

## 幂等与安全

- 额度发放以 `reference` 或 `idempotency_key` 幂等，重复提交只生效一次。
- 身份事件以 `event_id` 幂等，重复投递返回 `already_processed`。
- 虚拟 Key 只在创建响应中返回一次，列表与审计不保存明文。
- 消费端不得缓存、写入日志或返回到浏览器的主站 token、grant 或 Key 明文。

## 消费者接入

- ai-web：登录走主站 OIDC，BFF 用 grant 调用 `/me/ledger`、`/me/keys`；模型请求使用会话 Key。
- YCZX Code：用户创建个人 Key 后，直接用 `https://api.yanchuaner.cn/v1` 调用模型；额度、限流、审计都由 API 控制面负责。
- 后续 Agent：复用 YanCore grant 或个人 Key，不自行实现额度、账本或身份。
