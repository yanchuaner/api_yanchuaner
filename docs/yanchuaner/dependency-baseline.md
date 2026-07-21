# 依赖与构建基线

## 目的

本文件在功能实现前固定工具链、依赖真值和升级门禁。P0 不新增第三方库；Go 与前端依赖继续以现有锁文件为准，任何升级必须单独提交，不与业务功能混合。

## 真值文件

| 范围 | 唯一真值 | 说明 |
| --- | --- | --- |
| Go 工具链 | `go.mod`、`go.sum`、Dockerfile 的 Go 镜像 digest | 本地验证使用 Go 1.26.x；发布以固定镜像摘要为准 |
| 默认前端 | `web/package.json`、`web/default/package.json`、`web/bun.lock` | 使用 Bun 严格读取锁文件，不生成 pnpm/npm 锁文件 |
| Classic 前端 | `web/classic/package.json`、`web/bun.lock` | 必须在独立依赖安装阶段使用 `bun install --filter ./classic --frozen-lockfile` |
| 容器与系统包 | `Dockerfile`、`deploy/compose.yaml` | 镜像摘要是不可变真值；标签仅用于可读性 |
| 第三方许可 | `THIRD-PARTY-LICENSES.md`、`THIRD_PARTY_NOTICES.md`、`NOTICE` | 升级依赖时同步复核许可和 NOTICE |

## 构建顺序

发布构建以 Dockerfile 的三个隔离阶段为准：

1. 默认前端阶段执行完整锁文件安装并构建 `web/default/dist`。
2. Classic 阶段在干净文件系统中仅安装 classic workspace 并构建 `web/classic/dist`。
3. Go 阶段复制两个前端产物后再构建入口二进制。

`.dockerignore` 必须排除 `.local-tests`、`.gocache`、`.gomodcache`、`node_modules` 与本地产物。构建日志中的 context 不应包含这些目录；若上下文异常增长到 GiB 级，应立即停止构建并修正忽略规则。

不要在同一个 Windows `node_modules` 中通过反复切换 Bun `hoisted` / `isolated` linker 模拟发布构建。当前锁图同时包含默认前端的 `date-fns@4` 和 Classic 间接依赖的 `date-fns-tz@1.3.8` / `date-fns@2`；共享 hoist 会造成错误配对，而 isolated 安装会产生重复 peer 类型。Dockerfile 的隔离阶段是当前已声明的解决方案。

## 本地验证

```powershell
# 默认前端：类型检查与生产构建
cd web
bun install --frozen-lockfile
cd default
bun run typecheck
bun run build

# Classic：使用干净依赖目录或直接验证 Docker 阶段
docker build --target builder-classic -t api-yanchuaner/classic-build-check .

# 完整发布构建
docker build -t api-yanchuaner/new-api:local-check .
```

如果 Windows 本地安装残留导致 peer 类型重复，应移走忽略的 `node_modules` 后重新安装；不得通过改业务类型、关闭检查或提交临时锁文件掩盖依赖布局问题。

## 升级门禁

依赖升级 PR 必须同时提供：

- 新旧版本与 digest 对照、上游来源和许可证变化；
- 默认前端、Classic 前端、Go 二进制的独立构建结果；
- SQLite 与 PostgreSQL 迁移检查，MySQL 在发布支持时补齐；
- 安全公告检查、回滚 digest 和数据兼容说明；
- `THIRD-PARTY-LICENSES.md`、`THIRD_PARTY_NOTICES.md` 与版权矩阵更新。

P0 期间禁止使用可变 `latest` 作为发布依据，禁止在功能提交中顺带刷新全部依赖。
