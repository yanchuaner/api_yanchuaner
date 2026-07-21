# 燕中校友开发者 API

本仓库以 New API `v1.0.0-rc.21` 为控制面基线，为认证校友提供统一模型接口、公益额度、用量审计和未来的 BYOK 能力。New API 的原始项目身份、版权和许可文件均予以保留。

当前阶段是私有内测底座，不开放密码注册、匿名调用、公开充值、邀请返利、兑换码或任意上游代理。生产启用前必须完成授权归档、安全验收、额度对账和应急演练。

## 架构边界

- `yanchuaner.cn`：账号、邮箱验证和校友身份审核的唯一来源。
- New API：兼容网关、用户映射、模型路由前置控制和余额投影。
- 燕中 P0 模块：哈希虚拟 Key、公益额度不可变流水和请求级审计关联。
- LiteLLM：上游模型路由、供应商凭据和故障切换。
- PostgreSQL / Redis：控制面、网关和短效状态分别隔离。
- BYOK Vault：后续独立服务；用户真实密钥不进入普通渠道表。

## 开始开发

```powershell
.\scripts\generate-deploy-env.ps1
.\scripts\check-deploy-config.ps1
docker compose --env-file deploy/.env -f deploy/compose.yaml up -d --build
```

Docker Desktop 未运行时仍可执行配置检查，但不能启动服务。完整操作步骤见 [部署说明](deploy/README.md)，依赖顺序见 [依赖基线](docs/yanchuaner/dependency-baseline.md)，自主模块见 [P0 设计](docs/yanchuaner/p0-control-plane.md)，版权边界见 [来源矩阵](docs/yanchuaner/copyright-matrix.md)，上线判定见 [验收矩阵](docs/yanchuaner/acceptance.md)。

## 上游维护

```powershell
git fetch upstream --tags
git log --oneline main..upstream/main
```

升级必须先在独立分支合并并完成账单、鉴权、流式响应和回滚验证。不要自动跟随 `latest`。本地差异登记在 [PATCHES.md](PATCHES.md)。
