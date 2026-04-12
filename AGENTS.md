
## 构建和测试命令
```bash
# 后端
go build ./...
go test ./...
golangci-lint run

# 前端
pnpm install
pnpm run build
pnpm run lint
pnpm run dev
```

## 架构
- **项目类型**: Amp 反代管理系统 (全栈)
- **后端语言**: Golang
- **前端框架**: React + shadcn/ui
- **功能模块**: 用户管理、管理员界面、Amp 代理
- **参考实现**: `CLIProxyAPI/` (只读，包含 amp 代理核心实现)
- **入口文件**: TODO - 请指定主文件 (例如: main.go, src/main.tsx)

## 代码风格
- 遵循代码库中的现有模式
- 使用一致的命名规范 (根据语言选择 camelCase/snake_case)
- 显式处理错误，不要静默忽略
- 保持函数小而专注
- 前端内使用尽量少的描述性文字，保持简介性

## 环境注意事项
- **操作系统**: Windows，使用 PowerShell，不要用 `tail`/`head`/`grep` 等 Linux 命令
  - 用 `Select-Object -Last 30` 代替 `tail -30`
  - 用 `Select-Object -First 30` 代替 `head -30`
  - 用 `Select-String` 代替 `grep`
- **cwd 参数**: 必须是绝对路径，不要与工作区根目录重复拼接
  - 正确: `e:\MegAiTools\AMPManager\web`
  - 错误: `e:\MegAiTools\AMPManager\e:\MegAiTools\AMPManager\web`
- **前端目录**: `web/`，在此目录下运行 `pnpm run build` 等命令
- **本地 PostgreSQL**: 开发环境默认使用 `docker-compose.dev.yml` 启动 PostgreSQL，端口 `5432`，用户名 `postgres`，密码 `mysecretpassword`，数据库 `ampmanager`
- **开发脚本**: `dev.ps1` / `dev.sh` 会同时启动 PostgreSQL、Vite 热更新和 Air 热更新

## 备注
- 这是一个新项目，请随着代码库的发展更新此文件

## Playwright 浏览器工具

所有由Playwright MCP产生的文件都要放进./playwright-mcp

使用 Playwright MCP 工具查看和测试网页效果：

```bash
# 导航和截图
mcp__playwright__browser_navigate      # 打开 URL
mcp__playwright__browser_snapshot      # 获取页面快照（推荐用于交互）
mcp__playwright__browser_take_screenshot  # 截图保存

# 页面交互
mcp__playwright__browser_click         # 点击元素
mcp__playwright__browser_type          # 输入文本
mcp__playwright__browser_hover         # 悬停元素
mcp__playwright__browser_fill_form     # 填写表单

# 调试
mcp__playwright__browser_console_messages  # 查看控制台日志
mcp__playwright__browser_network_requests  # 查看网络请求
mcp__playwright__browser_close         # 关闭浏览器
```

## Context7 文档查询

使用 Context7 MCP 工具查询库和框架的最新文档：

```bash
# 1. 先解析库名获取 ID
mcp__context7__resolve-library-id  # 搜索库名，获取 Context7 兼容的库 ID

# 2. 使用库 ID 查询文档
mcp__context7__query-docs          # 查询具体 API 用法和示例
```

**使用示例：**
- 查询 React 文档：先 resolve "react"，再用 `/facebook/react` 查询
- 查询 shadcn 文档：先 resolve "shadcn"，再查询具体组件用法
- 查询 Go 标准库：先 resolve "golang"，再查询具体包用法

**注意：** 每个问题最多调用 3 次，优先使用此工具获取最新 API 文档。

## shadcn/ui 组件

使用 MCP 工具查询和添加 shadcn 组件：

```bash
# 查询可用组件
mcp__shadcn__search_items_in_registries  # 搜索组件
mcp__shadcn__list_items_in_registries    # 列出所有组件
mcp__shadcn__view_items_in_registries    # 查看组件详情

# 获取组件示例
mcp__shadcn__get_item_examples_from_registries  # 获取使用示例

# 添加组件
mcp__shadcn__get_add_command_for_items  # 获取添加命令
# 例如: npx shadcn@latest add button card
```

## Git 规范

### 主分支
- 主分支为 `main`，推送时使用 `git push origin master:main` 或切换到 main 分支

### Commit 格式
```
feat(api): 添加批量注册功能
fix(core): 修复 cookies 过期检测
refactor(ui): 优化任务列表渲染
docs: 更新 README
```
