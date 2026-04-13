# 本次改动报告

## 提交信息
- Commit: `e0378b7`
- Message: `feat(app): 优化概览设置日志与站点配置`

## 本次改了哪些部分

### 1. 概览页、管理概览页、使用量统计
- 卡片数字格式统一改为完整数字显示，不再使用 `K / M / B` 缩写。
- 数字增加千分位分隔。
- 余额、成本等金额展示同步做了整数位分组。
- 14 天趋势图由后端补齐缺失日期，前面没数据时自动补 `0`。
- 缓存命中率 3 个供应商面板统一高度，无数据时也保留固定占位。

### 2. 账户设置 / 计费设置
- “订阅额度使用情况”不再单独放一组横向进度条。
- 进度合并进“订阅状态”，改为每条额度一行的小圆环进度。

### 3. Amp 设置页
- 重排基础设置区域，减少内嵌卡片感。
- “网页搜索模式”从三块单选卡改成下拉选择。
- `Upstream URL / 显示余额提醒 / Upstream API Key / SOCKS5 代理` 重新排版对齐。
- 删除 SOCKS5 下方那句冗长说明。
- 映射规则编辑器压窄表头与列宽，去掉超宽横向滚动布局，修正表头对齐问题。

### 4. API Key 管理
- 创建 API Key 时支持“可选自定义”。
- 自定义规则:
  - 至少 16 位
  - 只允许字母和数字
- 已有 API Key 仍然不能编辑。
- 后端增加重复值校验和明确错误返回。

### 5. 请求日志
- 新增 `TTFB` 和 `TPS` 两列，位置在“延迟”左边。
- 只对流式请求显示。
- `TPS` 按 `输出 Tokens / (Duration - TTFB)` 计算。
- 非流式或无法计算时显示 `-`。

### 6. 用户管理
- 新增后端分页接口。
- 前端加分页器和每页条数控制。
- 删除最后一条导致当前页为空时，会自动回退上一页。

### 7. 系统设置 / 网站配置
- 系统设置新增第一个 Tab: `网站配置`。
- 新增“网站名称”设置项。
- 网站名称会影响:
  - 登录页品牌文字
  - 注册页品牌文字
  - 左上角品牌文字
  - 浏览器 Tab 标题

### 8. 后端与测试
- 新增站点配置读取/保存接口。
- 新增请求日志 `TTFB` 落库字段。
- 新增用户分页接口。
- 新增自定义 API Key 校验测试。
- 新增 14 天趋势补零测试。

## 改之前 vs 改之后

| 模块 | 改之前 | 改之后 | 重点对比什么 |
| --- | --- | --- | --- |
| 概览卡片数字 | 大数字会显示 `1.2K / 3.4M` | 全部显示完整数字并加千分位 | 账户余额、今日用量、近 7 天、近 30 天 |
| 计费状态余额 | 余额显示样式和其他完整数字不一致 | 金额统一按完整数字分组显示 | 概览页“计费状态”右侧余额 |
| 缓存命中率卡片 | 某供应商没数据时卡片更矮，视觉不齐 | 3 张卡保持同高，空态也有固定结构 | 概览页和管理概览页底部 3 张供应商卡 |
| 14 天趋势图 | 前几天没数据就直接缺点/缺日期 | 固定返回 14 天，缺失日期补 0 | “每日成本趋势”和“每日请求量”开头部分 |
| 订阅额度展示 | 独立一组横向进度条，信息比较散 | 合并进“订阅状态”，用小圆环表达占用 | 概览页计费状态、账户设置计费设置 |
| Amp 设置布局 | 右侧搜索模式是三块单选卡，整体块感偏重 | 搜索模式改下拉，字段按网格对齐 | URL、API Key、余额提醒、SOCKS5 的整齐度 |
| SOCKS5 区域 | 有一整句较长说明 | 说明删除，界面更干净 | Amp 设置页连接区域 |
| 映射规则编辑器 | 表格过宽，容易横向滚动，表头不稳 | 列宽收紧，表头和内容更齐 | 存在映射规则时的桌面布局 |
| API Key 创建 | 只能随机生成 | 可选手填自定义 Key | 创建弹窗新增输入框 |
| 请求日志 | 只有延迟，没有 TTFB/TPS | 多两列流式性能指标 | 流式日志记录 |
| 用户管理 | 一次性查全量用户，列表越大越重 | 后端分页 + 前端分页器 | 翻页、每页条数、删除后的页码处理 |
| 网站名称 | `AMPManager / AMP Manager` 分散写死 | 统一由系统设置驱动 | 登录、注册、左上角、浏览器标题 |

## 建议你重点看哪些效果

### UI 重点
- 概览页、管理概览页:
  - 数字是否都变成完整格式
  - 缓存命中率 3 张卡是否同高
  - 趋势图前半段是否有补零
- 账户设置 -> 计费设置:
  - 订阅状态里的圆环是否清晰
- Amp 设置:
  - 网页搜索模式是否改为下拉
  - 4 个关键字段是否明显更整齐
  - 映射规则是否不再出现难看的横向滚动
- 请求日志:
  - 流式请求是否出现 `TTFB / TPS`
  - 非流式是否显示 `-`
- 用户管理:
  - 是否可以正常翻页、改页大小、删除后页码回退
- 系统设置 -> 网站配置:
  - 保存网站名后，登录页、注册页、左上角、浏览器标题是否一起变化

### 后端重点
- 自定义 API Key:
  - 合法值能创建
  - 小于 16 位会报错
  - 带特殊字符会报错
  - 重复值会报冲突
- 请求日志:
  - `TTFB` 只对流式请求出现
  - `TPS` 分母确实是 `Duration - TTFB`

## 本地验证结果
- `go test ./...` 通过
- `go build ./...` 通过
- `pnpm run build` 通过
- `pnpm run lint` 未执行成功
  - 原因: 当前仓库缺少 ESLint 9 所需的 `eslint.config.js`
  - 这不是本次改动引入的语法错误

## 这次主要涉及的文件范围

### 后端
- `internal/repository/request_log.go`
- `internal/amp/request_trace.go`
- `internal/amp/connection_health.go`
- `internal/amp/log_writer.go`
- `internal/service/amp.go`
- `internal/handler/amp.go`
- `internal/repository/user.go`
- `internal/service/user.go`
- `internal/handler/user.go`
- `internal/service/system_config.go`
- `internal/handler/system.go`
- `internal/router/router.go`
- `internal/database/sqlite.go`

### 前端
- `web/src/pages/Overview.tsx`
- `web/src/pages/AdminOverview.tsx`
- `web/src/pages/AccountSettings.tsx`
- `web/src/pages/AmpSettings.tsx`
- `web/src/pages/APIKeys.tsx`
- `web/src/pages/RequestLogs.tsx`
- `web/src/pages/UserManagement.tsx`
- `web/src/pages/SystemSettings.tsx`
- `web/src/pages/Login.tsx`
- `web/src/pages/Register.tsx`
- `web/src/pages/Dashboard.tsx`
- `web/src/App.tsx`
- `web/src/components/ModelMappingEditor.tsx`
- `web/src/components/CircularProgress.tsx`

## 备注
- 本次代码已提交。
- `report.md` 是单独留在本地的说明文件，方便你按模块验收。
