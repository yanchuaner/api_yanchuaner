# 阶段 1：YanCore 主体凭证

更新日期：2026-07-19
状态：`IDENTITY_EXCHANGE_IMPLEMENTED`

## 目标

先建立燕中自己的业务协议，再接入 AI 工作台和 YCZX Code。`YanCore Subject Grant` 表示“某个已经由主站认证的用户，授权某个燕中应用在某个受众范围内，以有限 scope 工作一段时间”。它不是 New API Token，不是 LiteLLM Virtual Key，也不是 Open WebUI 会话 Cookie。

## v0 契约

| 项目 | 规则 |
| --- | --- |
| 签发 | API 会话或自主 AI Web BFF 持有的短期主站访问令牌；两条路径最终映射到同一 API 用户 |
| 主体 | JWT `sub=yc_user_<id>`，不携带邮箱、姓名或班级等多余资料 |
| 应用 | v0 仅允许已登记的 `ai-web`；新增应用必须修改策略并补测试 |
| 受众 | v0 固定 `yanchuaner-ai`，introspection 必须声明受众以阻止跨应用重放 |
| 权限 | v0 仅允许 `chat:read`、`chat:write` 的子集 |
| 有效期 | 协议上限 24 小时；v0 `ai-web` 策略硬限制为 15 分钟 |
| 撤销 | 数据库保存 JTI 哈希；撤销后 introspection 立即失败 |
| 密钥 | 独立 `YANCHUANER_SUBJECT_SIGNING_SECRET`，不复用 LiteLLM Master Key |
| 数据库 | `yan_core_subject_grants` 仅保存 JTI 哈希和最小审计字段，不保存完整 JWT |

## API

- `POST /api/yancore/grants/`：登录用户签发短期 grant；完整凭证只在本次响应返回。
- `GET /api/yancore/grants/`：登录用户查看自己的 grant 元数据，永不返回 JWT。
- `DELETE /api/yancore/grants/:id`：登录用户撤销自己的 grant。
- `POST /api/yancore/grants/introspect`：持有者以 `Authorization: Bearer <grant>` 请求校验主体、应用、受众、scope 和过期时间。
- `POST /api/yancore/subject-exchange`：AI Web BFF 以独立 Basic 客户端凭据提交短期主站访问令牌；API 只向固定 UserInfo 地址复验，并只映射已绑定 `yanchuaner` OAuth 的用户。

## 身份交换边界

- `subject_token` 只在服务端请求体中短暂存在，不写数据库、日志或浏览器可读状态；
- UserInfo 地址由部署环境固定，客户端不能提交 URL；HTTP 仅可通过显式本地开发开关启用；
- 交换客户端 Secret、主站 OIDC Client Secret 与 grant 签名 Secret 相互独立；
- 不按邮箱自动合并账户，也不通过交换接口创建 New API 用户；未绑定主体返回 403；
- 交换接口只能签发 `ai-web / yanchuaner-ai / chat:read chat:write`，TTL 不超过 15 分钟。

## 第三方隔离

New API 当前继续承载兼容网关和模型转发，但不拥有 `app`、`aud`、`scp` 的业务定义。LiteLLM 只接收经过控制面策略检查的内部请求。Open WebUI 适配阶段必须把 grant 转换为可验证的用户主体；在完成前，共享服务 Key 只能记独立服务账户。

## 阶段 1 验收

- 短 Secret、空应用/受众/scope、超 24 小时 TTL 均拒绝；
- 签发后的 JWT 不落库，数据库只出现 JTI 哈希；
- 错误受众、错误签名、过期和撤销 grant 均失败；
- 用户只能查看和撤销自己的 grant；
- 错误交换客户端、失效主站令牌、未认证角色、未绑定主体和停用 API 用户均拒绝；
- 重启后 grant 仍可验证，轮换 Secret 的行为有明确停机/迁移方案；
- API、AI、YCZX Code 使用同一契约测试，不复制上游实现。

## 回滚

先关闭 `YANCHUANER_SUBJECT_EXCHANGE_ENABLED`，AI Web 停止建立新会话；必要时再关闭 grant 签发入口。保留已签发记录和只读 introspection；AI/Agent 可回退到各自受限服务账户，但不得把共享账户调用解释为个人公益额度。数据库字段不删除，待新控制面替换后迁移。
