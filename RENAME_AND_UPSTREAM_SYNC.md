# kyllenios-core 重命名与上游同步总结

日期：2026-03-15

## 1. 文档范围

本文档总结本次项目重命名、远端调整、上游合并、验证以及父仓库子模块更新的完整结果，供后续维护和继续同步上游时参考。

## 2. 本次已完成事项

### 2.1 项目重命名

项目已从旧名重命名为 `kyllenios-core`。

已完成的内容：

- 将项目内容中的旧名替换为 `kyllenios-core`
- 重命名项目目录
- 重命名相关 service 文件和 runtime 路径
- 更新 Go module 路径
- 更新 Git 远端地址

本地路径变化：

- 旧路径：`/Users/money/project/subproject/<old-name>`
- 新路径：`/Users/money/project/subproject/kyllenios-core`

子项目重命名提交：

- `347c9b9f` 项目重命名提交

### 2.2 GitHub 仓库改名

GitHub 仓库已从：

- `https://github.com/ca0fgh/<old-name>`

改为：

- `https://github.com/ca0fgh/kyllenios-core`

当前子项目远端：

```bash
origin   https://github.com/ca0fgh/kyllenios-core.git
upstream <已配置的原始上游仓库地址>
```

### 2.3 上游关系已澄清

当前仓库 `ca0fgh/kyllenios-core` 在 GitHub 上是一个 fork。

真实上游关系已经接入为本地 `upstream` remote。

因此，后续标准 remote 关系应保持为：

- `origin` = 当前工作仓库：`ca0fgh/kyllenios-core`
- `upstream` = 原始上游仓库对应的本地 remote

### 2.4 上游 main 已合入

`upstream/main` 已经合并到子项目的 `main`。

子项目 merge commit：

- `54b9bf55` `Merge upstream/main`

合并过程中还顺手修了 3 个当前 fork 特有的兼容问题：

- `backend/internal/handler/gateway_handler_stream_failover_test.go`
- `backend/internal/repository/migrations_runner_extra_test.go`
- `backend/internal/repository/usage_log_repo_request_type_test.go`

原因分别是：

- 上游新增测试里还残留原始模块路径引用
- migration checksum 测试与当前本地 auto-fix 行为不一致
- repository 的 sqlmock 测试没有覆盖新增 endpoint 统计查询

### 2.5 父仓库子模块已更新

父仓库 `/Users/money/project` 已同步更新子模块路径和子模块指针。

父仓库相关提交：

- `9619576` 子模块重命名提交
- `dc22c97` `Update kyllenios-core submodule`

## 3. 验证结果

在子项目中已完成以下验证：

```bash
cd /Users/money/project/subproject/kyllenios-core/backend
go test ./...

cd /Users/money/project/subproject/kyllenios-core/frontend
pnpm typecheck
```

验证结果：

- backend 全量测试通过
- frontend 类型检查通过

当前仓库状态：

- 子项目工作区干净
- 父仓库工作区干净

## 4. 当前仓库状态

### 4.1 子项目

路径：

- `/Users/money/project/subproject/kyllenios-core`

分支：

- `main`

当前提交：

- `54b9bf55`

远端：

```bash
origin   https://github.com/ca0fgh/kyllenios-core.git
upstream <已配置的原始上游仓库地址>
```

### 4.2 父仓库

路径：

- `/Users/money/project`

分支：

- `main`

当前提交：

- `dc22c97`

子模块路径：

- `subproject/kyllenios-core`

## 5. 后续继续同步上游的标准流程

在子项目里执行：

```bash
cd /Users/money/project/subproject/kyllenios-core

git switch main
git pull --ff-only origin main
git fetch upstream
git merge upstream/main
```

合并完成后，必须额外做一次命名回归检查，确认上游带回来的 `sub2api` 名称是否已经继续替换为 `kyllenios-core`。

建议至少检查以下几类内容：

- Go module / import 路径中的 `sub2api`
- Docker、systemd、deploy 脚本里的旧服务名或旧二进制名
- 前端展示文案、站点名、默认配置值
- 测试中写死的旧模块路径、旧项目名、旧 endpoint 名称

可直接执行：

```bash
cd /Users/money/project/subproject/kyllenios-core

rg -n --hidden --glob '!.git' '([sS][uU][bB]2[aA][pP][iI])'
```

规则直接固定为：

- 与上游合并后，凡是回流进当前项目代码、文档、脚本、测试里的 `sub2api`，都替换为 `kyllenios-core`

如果 merge 成功且验证通过，再推送：

```bash
git push origin main
```

之后回到父仓库更新子模块指针：

```bash
cd /Users/money/project

git add subproject/kyllenios-core
git commit -m "Update kyllenios-core submodule"
git push origin main
```

## 6. 备注

- GitHub 上旧地址可能仍然可访问，这通常是仓库重命名后的重定向行为，不影响当前标准仓库名。
- 由于本仓库已经做过一次完整的旧名到 `kyllenios-core` 的重命名，后续再合并上游时，命名敏感文件发生冲突的概率会更高。
- 如果以后再遇到 merge 冲突，优先检查这些位置：
  - `go.mod`
  - deployment 脚本
  - service 文件
  - README / 文档
  - 写死旧模块路径或旧命名的测试
