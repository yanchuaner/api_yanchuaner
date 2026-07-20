# 内测部署

默认 Compose 栈只运行 New API 控制面、PostgreSQL 与 Redis，并通过 `yanchuaner-ai-core` 网络连接 `ai_yanchuaner` 中唯一的 LiteLLM 数据面。Redis 仅为主站 OAuth 在本机回环地址发布 `6380`，数据库不发布端口。独立验证时可使用 `--profile standalone` 启动本仓库自带的备用 LiteLLM。

## 启动

```powershell
.\scripts\generate-deploy-env.ps1
.\scripts\prepare-integrated-env.ps1
.\scripts\check-deploy-config.ps1
docker network inspect yanchuaner-ai-core *> $null
docker compose --env-file deploy/.env -f deploy/compose.yaml up -d --build
docker compose --env-file deploy/.env -f deploy/compose.yaml ps
```

- New API：`http://127.0.0.1:3101`
- LiteLLM 管理界面由 `ai_yanchuaner` 提供：`http://127.0.0.1:4000/ui`

生产环境由 Nginx 或等价反向代理终止 TLS。不得把 PostgreSQL、Redis 或 LiteLLM 管理端口直接暴露到公网。

`deploy/.env` 必须保持 `YANCHUANER_HASHED_KEYS_ENABLED=true` 与 `YANCHUANER_QUOTA_LEDGER_ENABLED=true`。前者只影响新 Key 的创建方式；后者启用已有余额的 opening balance 回填和公益额度流水路径。

阶段 1 的 `YANCHUANER_SUBJECT_GRANTS_ENABLED` 默认保持 `false`。完成数据库迁移、独立 `YANCHUANER_SUBJECT_SIGNING_SECRET` 备份和 AI Web 契约测试后再启用；该 Secret 不得复用 Session、Crypto 或 LiteLLM Master Key。

阶段 2C 的 `YANCHUANER_VIRTUAL_KEY_POLICY_ENABLED` 也默认保持 `false`。先以关闭状态迁移表，再在隔离数据库开启并审查历史哈希 Key 回填：已知 OpenAI/DeepSeek 模型生成活动策略，未知或通配模型保持禁用。多实例和预览流量必须连接 Redis；已配置 Redis 但故障时请求返回 503。完整步骤见 `docs/yanchuaner/phase-2-virtual-key-policy.md`。

## 首次初始化顺序

1. 在本机隧道中完成 New API root 初始化。引导脚本会生成 `NEW_API_ROOT_USER_ID` 与 `NEW_API_ROOT_ACCESS_TOKEN` 并仅写入忽略提交的 `deploy/.env`，供关闭密码登录后的重复部署使用。
2. 将系统名称设为“燕中 API”并使用极简前端；保持总注册开关开启（主站 OAuth 首次建号需要），但关闭密码登录、密码注册及除“燕中统一身份”外的登录提供商。已认证在校生、校友与教师首期获得 `$1.00 / ¥7.00`，邀请人与受邀人额外额度保持为零。
3. 在 LiteLLM 添加 OpenAI Platform API Project 与 DeepSeek 官方 API 渠道。
4. 创建仅允许已批准模型、带总预算和 RPM/TPM 限制的 LiteLLM 虚拟 Key。
5. 在 New API 新建一个自定义渠道，Base URL 使用 `http://litellm-gateway:4000`，Key 使用上一步的虚拟 Key，绝不使用 LiteLLM master key。
6. 在 New API 配置主站 OAuth，并为 Open WebUI 配置独立 OIDC 客户端：授权、Token、用户信息端点和访问策略见 `docs/yanchuaner/architecture.md`。
7. Open WebUI 共享服务 Token 使用 `OPENWEBUI_SERVICE_QUOTA` 有限总预算，脚本只会把旧的无限 Token 迁移一次，不会在每次重启时重置余额。创建公益额度分组后，使用测试用户完成预扣、结算、失败退款和并发请求对账。
8. 迁移后核对 `quota_ledger_entries`：每个已有用户至多一条 `opening_balance`，新 OAuth 用户有一条 `grant`；新虚拟 Key 的 `tokens.key` 以 `sha256:` 开头且数据库中不存在创建响应的明文。
9. 开启 Key 策略前核对 `yan_core_virtual_key_policies` 与修订表，验证模型/来源/预算/有效期、供应商拒绝、RPM/TPM/并发、Redis 故障和策略更新原因。

## 默认关闭项

以下能力在完成专项验收前不得启用：密码登录、密码注册、匿名调用、在线充值、支付回调、邀请返利、兑换码、任意 Base URL、用户 BYOK、自动从公益额度切换到用户凭据。总注册开关是 OAuth 自动建号的必要条件，不等同于开放本地账号体系。

## 停止与备份

```powershell
docker compose --env-file deploy/.env -f deploy/compose.yaml down
```

不要在存在有效数据时执行 `down -v`。备份必须同时覆盖两个 PostgreSQL 数据库、Redis AOF、New API 配置和密钥管理系统；备份中不得包含可直接阅读的上游密钥。

回滚时不删除额度、策略或修订表。先冻结调用并记录最后流水 ID，再回滚固定镜像；紧急关闭 Key 策略会失去独立供应商和每 Key 速率限制，只能在调用已冻结或另有限流保护时使用。恢复后按 request ID 核对余额、流水、用量日志和供应商账单。
