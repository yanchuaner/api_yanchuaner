# 燕中 API

燕中 API 是燕中生态的统一模型 API 控制面，面向主站已认证的在校生、校友与教师提供 OpenAI 兼容接口。它负责用户 API Key、模型权限、公益额度、逐请求用量、限流与审计；LiteLLM 负责上游路由和成本核对，两者不重复记账。

- 预览入口：`https://api.yanchuaner.cn`
- 本地入口：`http://localhost:3101`
- 兼容接口：`https://api.yanchuaner.cn/v1`
- 当前阶段：2026 燕中生态暑期预览

## 暑期预览定位

这次更新不是简单的 `v2` 或 `v3`。暑期结束目标是让认证成员从主站登录、领取有限公益额度、创建 API Key，并在燕中 AI、YCZX Code 与后续 Agent 中完成真实模型调用。正式版计划在大二上学期持续一个学期，根据授权范围、账单对账、故障演练、用户反馈和实际成本不断完善。

预览版坚持三条原则：

1. **唯一身份源**：关闭本地密码登录与注册，只接受 `yanchuaner.cn` 主站 OAuth。
2. **账单可解释**：额度同时展示美元与人民币，固定参考汇率为 `1 USD = 7 CNY`；每次调用记录模型、Token 与费用。
3. **能力最小化**：用户端只保留主页、控制台、文档、公告与登录；渠道、模型、用户和审计能力留给管理员。

## 当前能力

- 已认证在校生、校友与教师通过“燕中统一身份”登录。
- 主站管理员同步为燕中 API 管理员；其他 OAuth 提供方不能通过角色字段提权。
- 新成员首期公益额度为 `$1.00 / ¥7.00`。
- 用户按应用/场景创建和撤销虚拟 Key；新 Key 只展示一次，服务端只保存哈希与脱敏片段。
- 公益额度的赠送、预扣、结算、退款和管理员调整写入不可变流水，余额字段只作兼容投影。
- 管理员定向增减或覆盖额度时必须填写原因，并记录操作者、目标用户、金额与原因。
- Open WebUI 通过受限服务 Key 调用燕中 API；未来 Agent 使用同一 `/v1` 接口。
- LiteLLM 连接获授权的 OpenAI、DeepSeek 等上游；燕中额度流水是已迁移公益额度路径的业务真值，New API 保留兼容网关和余额投影。

## 调用链

```text
yanchuaner.cn 统一身份
        |
        v
燕中 API：用户 / API Key / 公益额度 / 用量账本 / 审计
        |
        v
LiteLLM：模型路由 / 重试 / 上游成本核对
        |
        +--> 获授权的 OpenAI Platform API Project
        +--> DeepSeek 官方 API
```

## 用户流程

1. 在主站完成邮箱与成员身份认证。
2. 在燕中 API 选择“燕中统一身份”登录。
3. 在控制台查看 `$ / ¥` 双币额度并创建 API Key。
4. 使用内部文档中的 Endpoint、Key 和模型名接入应用。
5. 在用量日志中逐条核对请求与费用。

## 本地部署

准备 Docker Desktop，并确保主站与 `ai_yanchuaner` 配置可用：

```powershell
cd C:\Dev\yanchuaner\api_yanchuaner
.\scripts\generate-deploy-env.ps1
.\scripts\bootstrap-integrated-stack.ps1
```

集成栈入口：

- 主站：`http://localhost:3000`
- 燕中 API：`http://localhost:3101`
- 燕中 AI：`http://localhost:3001`
- LiteLLM 管理端：`http://localhost:4000/ui`

详细部署说明见 [deploy/README.md](deploy/README.md)，系统边界见 [现状与风险](docs/yanchuaner/current-state-and-risks.md)，自主模块见 [P0 设计](docs/yanchuaner/p0-control-plane.md)，阶段 1 自主协议见 [YanCore 主体凭证](docs/yanchuaner/phase-1-yancore-subject-grant.md)，依赖与构建约束见 [依赖基线](docs/yanchuaner/dependency-baseline.md)，上线门槛见 [验收矩阵](docs/yanchuaner/acceptance.md)。

## 安全边界

- 不开放匿名调用、密码登录、密码注册、公开充值、邀请返利与任意 Base URL。
- 不建设账号池，不把消费级订阅账号转换为 API，不绕过上游授权或限制。
- 上游真实密钥不写入 Git、普通日志、前端或用户渠道配置。
- BYOK 仍属于后续专项，不在凭据保险库和审计完成前开放。
- 自动化 root 管理令牌只保存在被 Git 忽略的本地 `deploy/.env`。
- 旧明文 Token 必须通过“新建哈希 Key → 验证 → 撤销旧 Key”轮换，不能宣称已自动迁移。

## 开源与来源

本仓库是 New API 的 AGPLv3 修改分发，必须保留 `LICENSE`、`NOTICE`、版权头、界面署名和上游链接。燕中原创文件、授权修改、保留依赖与计划替换范围见 [版权与来源矩阵](docs/yanchuaner/copyright-matrix.md)，B 到 C 的验收与回滚见 [迁移清单](docs/yanchuaner/migration-b-to-c.md)。

## 路线图

### 暑期预览

- 完成主站 SSO、API Key、双币额度、逐请求账单与管理员定向福利。
- 完成燕中 AI 和 YCZX Code 的真实接入。
- 验证预扣、结算、失败退款、并发耗尽、限流、备份与恢复。
- 以少量认证成员开展灰度测试，不公开售卖。

### 大二上学期正式版准备

- 根据真实账单完善模型组合、预算策略和告警。
- 完成同能力多渠道故障切换与供应商成本对账。
- 评估合规 BYOK、凭据保险库和明确的资金来源选择。
- 完成生产监控、故障演练、撤销流程与长期运营规则。

## 相关项目

- 主站与统一身份：`web_yanchuaner`
- AI 网页工作台与 LiteLLM：`ai_yanchuaner`
- 科创教程与共建：`lab_yanchuaner`
- 微信小程序：`mp_yanchuaner`
- Agent 产品：`yczx_code`

项目级关系以 `C:\Dev\yanchuaner\docs\燕中生态项目关系.txt` 为准。
