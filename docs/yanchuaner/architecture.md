# 燕中 API 架构

## 目标

首版只解决一条完整链路：已在主站完成邮箱验证和身份审核的在校生、校友与教师，通过主站 OAuth 登录 New API，领取受限开发者 Token，消费公益额度，经 LiteLLM 调用获授权的 OpenAI 或 DeepSeek 官方 API。Open WebUI 使用独立 OIDC 客户端登录同一身份源，不维护第二套公开注册体系。

```text
认证成员 / Agent
       |
       v
New API 控制面  ----->  控制面 PostgreSQL / Redis
       |
       | 专用受限虚拟 Key
       v
LiteLLM 数据面  ----->  网关 PostgreSQL
       |
       +-----> OpenAI Platform API Project
       +-----> DeepSeek official API
```

## 责任边界

| 组件 | 唯一职责 | 不承担 |
| --- | --- | --- |
| 主站 | 登录、邮箱验证、成员身份审核、停用账号 | 模型密钥与消费账本 |
| New API | 用户 Token、模型权限、公益额度、用户侧消费账本 | 保存真实 BYOK、供应商故障切换 |
| LiteLLM | 上游凭据、路由、重试、供应商成本核对 | 面向用户的余额真值 |
| BYOK Vault | 后续加密保存和按用户解密凭据 | 用户额度结算 |

New API 是用户余额和扣费的唯一账本。LiteLLM 的费用数据只用于供应商成本核对，不得再次扣减用户余额。

## 主站 OAuth 契约

生产客户端使用固定回调地址和机密客户端：

- Authorization endpoint：`https://yanchuaner.cn/api/oauth/authorize`
- Token endpoint：`https://yanchuaner.cn/api/oauth/token`
- User info endpoint：`https://yanchuaner.cn/api/oauth/userinfo`
- Scope：`openid profile email`
- Subject：主站不可变的用户 UUID

主站只返回 `sub`、`preferred_username`、`name`、`email`、`email_verified` 和粗粒度 `role`。`role` 仅为 `admin`、`alumni`、`student`、`teacher` 之一；不返回毕业班级、联系方式、城市等成员资料。授权码 60 秒过期、只能消费一次并绑定 `client_id` 与精确 `redirect_uri`；访问令牌 5 分钟过期。

浏览器授权端点使用主站公开地址；Docker 中的 New API 和 Open WebUI 必须通过容器可达的主站内部地址兑换令牌和读取用户信息。开发环境使用 `host.docker.internal:3000`，生产环境应统一使用受 TLS 保护的正式域名。New API 与 Open WebUI 使用不同客户端密钥，任一密钥泄露都可以单独轮换。主站 OIDC ID Token 使用持久化 RSA 私钥进行 RS256 签名，发现文档的 JWKS 只发布公钥；不得在重启时临时生成新密钥。

New API 中配置字段映射：用户 ID `sub`、用户名 `preferred_username`、显示名 `name`、邮箱 `email`。首期 `NEW_USER_INITIAL_QUOTA=500000`，对应 `$1.00 / ¥7.00`；系统固定按 `1 USD = 7 CNY` 同时展示双币金额。Open WebUI 使用独立的有限共享 Token，不能设置无限额度，也不能在部署重启时自动补满。

主站 `admin` 身份在名为 `yanchuaner` 的受信任 OAuth 提供方内同步为 New API root；普通认证成员同步为普通用户。其他通用 OAuth 提供方即使返回同名字段，也不能赋予管理员权限。管理员增减或覆盖额度必须填写原因，金额、目标用户、操作者与原因写入管理审计日志。

自定义 OAuth 访问策略应再校验主站返回的角色：

```json
{
  "logic": "and",
  "conditions": [
    { "field": "role", "op": "in", "value": ["alumni", "student", "teacher", "admin"] }
  ]
}
```

New API 的总注册开关必须保持开启，否则首次 OAuth 登录无法创建本地映射用户；同时必须关闭密码登录、密码注册、其他 OAuth 提供商和邀请额度。这个组合只允许通过主站身份审核的 OAuth 用户自动建号。部署脚本首次初始化后生成仅存于忽略提交的 `deploy/.env` 的 root 管理令牌，后续配置维护不依赖已关闭的密码登录。

## BYOK 阶段

BYOK 不复用 New API 普通渠道表。Vault 只向业务层返回凭据引用，管理员界面显示供应商、尾号、创建时间和状态，不显示明文。请求时由 Broker 根据已认证用户和资金优先级在内存中解密，日志、错误、追踪和备份均不得出现密钥。

首版先预留 `credential_id`、`provider`、`status` 和资金来源概念，不创建用户上传密钥入口。只有公益额度链路稳定后才实现 BYOK。
