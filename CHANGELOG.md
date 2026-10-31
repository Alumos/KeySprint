# Changelog

KeySprint 遵循 [Semantic Versioning](https://semver.org/)，所有面向用户的重要变化都记录在此文件中。

## [Unreleased]

### Planned

- 后续变更先进入本节，发布时再归入对应版本。

## [2.0.0] - 2026-10-31

### Added

- **AI 一键生成范文**：文章管理新增「AI 生成」，按主题、体裁（记叙文/说明文/散文/童话/科普）与字数范围生成中文打字范文，生成后直接写入文章库并标记来源为「AI 生成」（新增 `articles.source`，旧数据自动归为手动上传）。
- 后台「系统配置 → AI 文章生成」：可设置 OpenAI 兼容接口地址、API 密钥（只存数据库，页面仅回显末 4 位，留空不覆盖）、默认模型（默认 `gpt-6-luna`）与思考强度（默认 `medium`），并支持**探测上游模型列表**。
- 上游不支持 `reasoning_effort` 时自动降级重试一次，兼容更多第三方 AI 网关。
- 新增环境变量 `AI_BASE_URL` / `AI_API_KEY` / `AI_MODEL` / `AI_REASONING_EFFORT` 作为初始默认值，后台配置优先。

### Changed

- **文章管理页重构**：合并「我的文章」与「共享文章库」为单一「文章库」，支持按上传教师、文章来源筛选，按导入时间或字数排序，并分页（每页 10 条），页面不再无限变长；表格改为整宽展示，操作列完整可见。

## [1.8.0] - 2026-10-31

### Added

- Windows 便携包（免 Docker 局域网部署）：CI 在原生 amd64 的 `windows-latest` 构建 `keysprint-<版本>-windows-x64.zip`（CGO 关闭，无 QEMU/arm64），打 Tag 时自动附到 GitHub Release；每次 push 也作为 Actions artifact 保留 30 天；包含启动、停止、防火墙、开机自启脚本与中文部署说明。
- Windows 包为**扁平结构**：zip 条目从根目录开始（`keysprint.exe`、`web/`…），解压一次即可运行；Actions artifact 上传的是打包目录内容，下载后同样只解压一次。
- 新增 [docs/LAN-DEPLOY.md](docs/LAN-DEPLOY.md)：机房/教室 Windows 局域网部署指南（备份、升级、多教师共用、常见问题）。
- 「复制比赛编号」在非安全上下文（`http://<内网IP>`）下回退 `execCommand`，局域网部署下仍可复制。

### Changed

- 容器镜像改为 `linux/amd64` 单架构，CI 移除 QEMU；构建更快，部署目标（VPS/学校服务器）均为 amd64。

## [1.7.0] - 2026-10-31

### Added

- 后台新增「系统配置」：网站标题、副标题与 ICP 备案号可在线修改，保存后立即生效；备案链接固定跳转 https://beian.miit.gov.cn/，项目不再内置特定备案号。
- 竞赛历史与成绩明细表支持分页（每页 10 条，显示总条数与页码，可上一页/下一页，条数变化自动收敛）。
- 前端启动时从 `/api/site` 读取站点外观，侧栏、登录页、赛场顶栏、浏览器标题与页脚统一使用配置值。

## [1.6.0] - 2026-10-31

### Added

- 竞赛创建支持「文本选项」多选列表：拼音提示、去除符号、符号统一、去除换行；按竞赛以 JSON 持久化，旧数据与旧客户端继续兼容。
- 教师 WebSocket 改用 90 秒一次性 ticket 鉴权；新增 `ALLOWED_ORIGINS` 配置，用于 CDN 回源 Host 与页面域名不一致的部署。
- 哈希静态资源（`/assets/*`）返回一年 immutable 缓存头，`index.html` 返回 no-cache，便于 CDN 正确升级。

### Changed

- 成绩提交接口要求携带比赛编号，比赛停止后仅保留 3 分钟宽限；停止后不再长期接受改分。
- 公开文章与公开名单接口要求有效比赛编号；导出改为带鉴权的 fetch，不再把会话 token 放进 URL。
- 在线名单与成绩事件只广播给教师连接；WebSocket 写失败立即断开并清理，避免死连接残留。
- 赛场输入缓存改为 400ms 防抖写入，并在页面卸载时兜底保存。

### Fixed

- 修复未提供 requestId 时第二场竞赛创建失败（空字符串命中 request_id 唯一索引）。
- 修复重置密码时数据库错误可能导致空指针的问题；关键写操作的错误不再静默忽略。
- 修复首次初始化可被并发请求抢注出多个管理员的问题。
- 修复 ClassOrbit 请求会跟随重定向、可能把共享密钥转发到其他主机的问题。

### Security

- `/ws` 升级前校验 Origin 与来源凭证（教师 ticket / 学生比赛编号），拒绝未认证连接。
- `JWT_SECRET` 未设置时改用进程内随机密钥，移除公开默认值。
- 移除 `?token=` 会话凭证回退；成绩、文章、名单的公开读取不再可被枚举。

## [1.5.0] - 2026-09-17

### Added

- 长文章打字时自动跟随当前行滚动，返回前文修改时同样跟随光标，并为输入法候选框预留空间。
- 新增输入、跨段落编辑、Unicode 字符、计分和滚动边界回归测试，并接入发布流水线。

### Changed

- 缓存文章字符节点和输入统计，减少计时刷新时的重复渲染与普通追加输入时的全文扫描。
- 输入缓冲区仅保留实际输入字符；成绩请求直接构造一次，保持旧缓存、接口及计分规则兼容。
- 关闭拼音提示时跳过拼音计算，离开赛场时清理待执行的 WebSocket 重连。

### Fixed

- 修复中间插入、删除后空值占位导致已输入字数虚增的问题。
- 修复完成全文时最后一个字可能未计入提交成绩的问题。
- 本地缓存不可用或 JSON 损坏时不再中断练习流程。

## [1.4.0] - 2026-09-04

### Fixed

- 为竞赛创建增加前端提交锁和服务端幂等校验，避免连续点击或请求重发生成两个比赛编号。
- 正确率统一为“正确字数 / 已输入字数”，速度统一为“每分钟正确字数”，且由后端根据原始计数计算。

### Changed

- 成绩表仅持久化正确字数、已输入字数和用时，移除重复的正确率与速度字段；旧数据库启动时自动无损迁移。
- 成绩明细与导出明确区分“已输入字数”和范文篇幅，并减少竞赛实时页面与成绩删除后的重复请求。

## [1.3.0] - 2026-08-31

### Changed

- 全站改用统一楷体字体栈，并与 ClassOrbit 共用 11–20px 的界面字号层级。
- 对齐侧栏、顶栏、按钮、表单、表格、卡片与弹窗等主要界面的排版尺度。

## [1.2.0] - 2026-08-30

### Added

- 名单来源后台新增“保存并测试连接”，可在写入配置后只读验证班级数与学生数。
- ClassOrbit 条件同步支持 ETag，远端名单未变化时跳过 SQLite 重复写入。

### Changed

- 完整显示 ClassOrbit 返回的鉴权、教师用户名等具体错误，减少联调排查成本。
- 使用 ClassOrbit 稳定班级 ID 处理改名，即使学生名单全部变化也能保持历史成绩关联。
- 同机 VPS 配置默认使用 `ghcr.milu.moe` 加速拉取镜像。

## [1.1.0] - 2026-08-30

### Added

- 管理后台新增班级名单来源配置，可在 Excel 手工导入与 ClassOrbit 接口之间切换。
- 班级记录各自的名单来源，接口班级与手工班级可以同时保留并清晰区分。
- 新增同机 VPS 部署配置，默认连接宿主机 `8080` 端口的 ClassOrbit，并将 KeySprint 发布到 `8081`。

### Changed

- ClassOrbit 同步仅停用远端已删除的接口班级，不再影响 Excel 手工导入的兜底班级。
- ClassOrbit 模式要求有效的 HTTP(S) 服务地址与共享 Bearer 密钥，错误配置会在保存时直接提示。

## [1.0.0] - 2026-08-30

### Added

- 教师文章库、班级名单、竞赛控制、教师账号与系统配置工作台。
- 比赛编号入场、WebSocket 统一发令、中文输入法组合输入和逐字拼音提示。
- 输入纠错、刷新恢复、迟到练习隔离、自动停赛与成绩择优保存。
- 成绩明细、个人与班级趋势、CSV 和 Markdown 导出。
- ClassOrbit 班级名单同步与稳定班级 ID 关联。
- SQLite WAL 持久化、数据库结构升级与 Docker Compose 部署。
- GitHub Actions 自动测试、双架构 GHCR 镜像、SBOM 与构建来源证明。

### Security

- bcrypt 密码存储、限时 JWT、管理员权限隔离和独立的班级同步密钥。
- scratch 非 root 运行镜像、只读根文件系统、应用内健康检查和最小容器权限。

[Unreleased]: https://github.com/Alumos/KeySprint/compare/v2.0.0...HEAD
[2.0.0]: https://github.com/Alumos/KeySprint/releases/tag/v2.0.0
[1.8.0]: https://github.com/Alumos/KeySprint/releases/tag/v1.8.0
[1.7.0]: https://github.com/Alumos/KeySprint/releases/tag/v1.7.0
[1.6.0]: https://github.com/Alumos/KeySprint/releases/tag/v1.6.0
[1.5.0]: https://github.com/Alumos/KeySprint/releases/tag/v1.5.0
[1.4.0]: https://github.com/Alumos/KeySprint/releases/tag/v1.4.0
[1.3.0]: https://github.com/Alumos/KeySprint/releases/tag/v1.3.0
[1.2.0]: https://github.com/Alumos/KeySprint/releases/tag/v1.2.0
[1.1.0]: https://github.com/Alumos/KeySprint/releases/tag/v1.1.0
[1.0.0]: https://github.com/Alumos/KeySprint/releases/tag/v1.0.0
