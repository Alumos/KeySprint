# KeySprint 项目说明（架构 / 开发 / 部署）

> 面向本地开发与自托管部署的中文说明。当前版本：`VERSION` 文件（1.7.0），代码基线 `main`。
> 部署形态：一台国内 VPS 上 Docker 单容器（HTTP 某端口）→ 腾讯云 EdgeOne CDN 加速并终结 HTTPS → HTTP 回源。

## 1. 一句话架构

单进程 Go 程序同时提供 JSON API、WebSocket 实时通道和前端静态站点；数据落在进程内嵌的 SQLite（WAL）；前端是 React 19 + Vite 构建的 SPA。**没有独立的数据库、Redis 或前端容器。**

```
浏览器 ──HTTPS──> EdgeOne CDN ──HTTP 回源──> VPS:端口 ──> keysprint 容器
                                                  ├─ /api/*   JSON API
                                                  ├─ /ws      WebSocket（发令、在线名单）
                                                  └─ /*       前端静态文件 + SPA 回退
                                                  └─ /data/keysprint.db  SQLite（named volume）
```

## 2. 技术栈

| 层 | 选型 |
| --- | --- |
| 后端 | Go 1.24（标准库 `net/http` + Go 1.22 的 `"METHOD /path"` 路由），无 Web 框架 |
| 数据库 | SQLite（`modernc.org/sqlite` 纯 Go 驱动，无 CGO），WAL 模式 |
| 实时 | `gorilla/websocket`，内存 hub，比赛 ID 即房间 |
| 鉴权 | bcrypt + HS256 JWT，教师才有账号（`is_admin` 区分管理员） |
| 前端 | React 19、react-router-dom 7、TypeScript 5.8（strict）、Vite 6、Radix UI、recharts、pinyin-pro |
| 产物 | 单个 Go 静态二进制 + `dist` 静态目录；`scratch` 镜像、非 root 65532 |

## 3. 目录与文件职责

### 后端 `backend/`

| 文件 | 职责 |
| --- | --- |
| `main.go` | 入口、环境变量、SQLite 打开与 pragma、内嵌 schema 执行、增量迁移、路由装配、静态站点与缓存头、比赛停止定时器 |
| `auth.go` | bcrypt、JWT 签发/校验（会话 token 与 WebSocket 一次性 ticket 分 scope）、鉴权中间件 |
| `websocket.go` | `/ws` 升级前的来源校验（Origin / ALLOWED_ORIGINS）、教师 ticket 与学号+比赛编号鉴权 |
| `hub.go` | 连接/房间模型、广播（比赛事件给全房间，名单与成绩事件只给教师）、心跳、断线清理 |
| `handlers_auth.go` | 初始化、登录、教师 CRUD、改密/重置、`POST /api/ws-ticket` |
| `handlers_articles.go` | 文章 CRUD、共享文章库、按比赛编号公开读文章 |
| `handlers_competitions.go` | 竞赛创建（编号生成、requestId 幂等、文本选项）、开始/停止、在线参赛者 |
| `handlers_results.go` | 成绩提交（比赛编号校验 + 结束宽限期）、查询、排名、导出 |
| `handlers_classes.go` | 班级/学生、Excel 导入、按比赛编号公开名单、学生与班级分析 |
| `handlers_integration.go` | ClassOrbit 集成配置读写（存 `app_meta`，优先于环境变量） |
| `handlers_site.go` | 系统外观配置：网站标题、副标题、ICP 备案号（存 `app_meta`，公开只读 + 管理员写入） |
| `class_system.go` | ClassOrbit 名单同步（Bearer 共享密钥、ETag、全量对账、软停用） |
| `text_options.go` | 文本选项 JSON 模型与旧数据兼容解析 |
| `util.go` | JSON 响应、错误体、请求体限制、路径参数 |
| `schema.sql` | 初始 schema（`go:embed`，每次启动幂等执行） |
| `main_test.go` | 集成测试（初始化/登录/竞赛/成绩/ClassOrbit/迁移/文本选项/安全边界） |

### 前端 `frontend/src/`

| 文件 | 职责 |
| --- | --- |
| `main.tsx` / `App.tsx` | 入口、BrowserRouter、7 条路由（页面全部懒加载） |
| `lib/api.ts` | `api()` 统一请求（自动带 `x-admin-token`、401 清 token）、`wsURL()` |
| `lib/types.ts` | 领域类型（Article/Competition/Result/ClassItem/Student/Teacher） |
| `lib/site.tsx` | 站点外观：`SiteProvider` 启动时拉取 `/api/site`，提供标题/副标题/备案号与刷新方法 |
| `lib/textOptions.ts` | **文本选项引擎**：类型、默认值、菜单元数据、文章变换、判定等价、旧数据归一 |
| `lib/typing.ts` | 打字缓冲区纯函数：稀疏 buffer、插入/退格、计数（可传判定器）、滚动跟随 |
| `components/ui.tsx` | Button/Input/Field/Panel/Badge/Empty/Modal/AppSelect/StatStrip/**OptionsMenu** |
| `pages/Setup.tsx` | 首次管理员初始化 |
| `pages/TeacherLogin.tsx` | 教师登录（token 存 sessionStorage） |
| `pages/Admin.tsx` | 工作台外壳：侧栏 6 模块、改密、退出、toast |
| `pages/StudentLogin.tsx` | 学生入口：比赛编号 → 班级/名单选人或手填 |
| `pages/Compete.tsx` | 赛场：IME 输入、计时、WebSocket、断点恢复、提交重试 |
| `pages/admin/*.tsx` | 文章、班级、竞赛、成绩、教师、名单来源、系统配置七个模块 |

### 根目录

| 文件 | 职责 |
| --- | --- |
| `Dockerfile` | 4 阶段构建：前端 dist → Go 交叉编译 → 证书 → `scratch` 运行 |
| `docker-compose.yml` / `compose.deploy.yaml` / `compose.vps.yaml` | 本地源码构建 / 标准服务器 / 与同机 ClassOrbit 共存 |
| `Makefile`、`scripts/check-version.sh` | 本地命令与版本门禁 |
| `.github/workflows/publish-container.yml` | 测试 → 双架构镜像 → GHCR → SBOM/provenance → Release |
| `VERSION`、`CHANGELOG.md`、`RELEASING.md` | 版本唯一源与发布流程 |

## 4. 数据模型

8 张表，全部由 `schema.sql` 幂等创建，启动时再跑增量迁移：

| 表 | 关键字段 | 说明 |
| --- | --- | --- |
| `teachers` | username（唯一、忽略大小写）、password_hash、display_name、is_admin | 只有教师是账号 |
| `articles` | teacher_id、title、content | 作者删除后文章保留（SET NULL） |
| `teacher_articles` | (teacher_id, article_id) | 文章库「选用」关系 |
| `competitions` | access_code（唯一）、request_id、status、time_limit、pinyin_enabled、**text_options**、started_at、stopped_at | 比赛主表；`text_options` 为 JSON |
| `results` | competition_id、student_id、student_name、correct_count、typed_count、duration、class_id | 唯一键 (competition_id, student_id)；正确率/速度**不落库** |
| `classes` | teacher_id、name、remote_id、source(manual/classorbit)、active | 接口同步的班软停用 |
| `class_students` | class_id、student_id、student_name | 唯一 (class_id, student_id) |
| `app_meta` | key、value | ClassOrbit 集成配置等键值 |

**派生字段**：`accuracy = correct_count / typed_count`，`speed = correct_count × 60 / duration`，查询时计算；`class_id` 是成绩归属快照（老数据启动时按「教师 + 学号 + 姓名唯一命中」回填）；`student_count`、`article_title` 等为查询聚合。

**迁移策略**：无迁移版本表，靠「`CREATE IF NOT EXISTS` + 忽略错误的 `ALTER TABLE` + 启动时补索引」。新增列必须同时在 `schema.sql` 与 `migrateSchema` 里各写一次（前者给新库，后者给老库）。

## 5. 后端行为

### 鉴权
- 登录 `POST /api/admin/login` → HS256 JWT（7 天）；之后所有受保护接口带 `x-admin-token`。
- `JWT_SECRET` 未设置时启动会生成进程内随机密钥并打印警告（重启后需重新登录），不再使用公开默认值。
- WebSocket 不能用请求头：教师先 `POST /api/ws-ticket` 换 90 秒一次性 ticket，再连 `/ws?ticket=...`；学生用「比赛编号 + competitionId + 学号」连 `/ws?code=...`。
- 学生不是账号，身份由前端自报；成绩提交必须携带比赛编号（`accessCode`），且比赛停止后只留 3 分钟宽限。

### 主要接口（分组）
- 公开：`/api/health`、`/api/setup/*`、`/api/admin/login`、`/api/competition/by-code/{code}`、`/api/articles/{id}?code=`、`/api/classes/by-teacher/{id}?code=`、`/api/classes/{id}/students/public?code=`、`POST /api/results`（带 accessCode）、`GET /ws`（带凭证）。
- 教师（需登录）：session、教师管理（管理员）、文章/文章库、竞赛创建/开始/停止/删除、在线名单、成绩查询/删除/导出、班级导入/同步、集成配置、分析。
- 统一错误体：`{"success":false,"error":"中文提示"}`。

### 比赛生命周期
`waiting → running`（记录 started_at、广播 `competition:start`、安排 `time_limit+10s` 的停止定时器）`→ stopped`（手动/超时，广播 `competition:stop`）。进程重启时扫描 running 比赛重建或立即置停。

### 成绩与统计
提交时校验比赛编号与状态、字数不超过范文长度，夹取 correct/duration；`(competition_id, student_id)` 唯一，只有「速度更快、或同速正确率更高」才覆盖；排名 = 速度降序 → 正确率降序 → 正确数降序 → id。班级分析按班级名聚合（同名班会合并）。

### ClassOrbit 同步
服务端到服务端：用当前教师用户名 + 共享 Bearer 密钥请求 `CLASS_SYSTEM_URL + CLASS_SYSTEM_PATH`，支持 ETag 304；同步为权威全量对账：按 remote_id → 班名 → 学生重叠度过半匹配本地班，远端消失的接口班 `active=0`，手工班不受影响，学生整班替换。不跟随重定向（避免密钥被转发）。

## 6. 前端行为

- 路由：`/`→`/student`、`/setup`、`/admin`（登录）、`/admin/dashboard`、`/student`、`/student/compete`。Admin 内部模块是 state 切换，不改变 URL。
- 状态：无全局 store；登录 token 与学生身份放 `sessionStorage`，其余为页面局部 state。
- 打字引擎：`typing.ts` 用稀疏对象记录「文章下标 → 已输入字符」，换行不可输入；插入/退格会整体右移/左移已输入内容；`countInput` 支持传入判定器。
- 赛场：`Compete.tsx` 处理 IME 组合事件、隐藏输入框、光标移动、250ms 计时、`localStorage` 断点恢复（400ms 防抖写入）、提交失败指数退避重试。
- 计分口径：正确率 = 正确字数 ÷ 已输入字数；速度 = 正确字数 ÷ 用时（分钟）；进度只用于显示。

## 7. 文本选项（本次新增）

竞赛创建处的「拼音提示」开关升级为「文本选项」按钮，点开是固定宽度多选列表；四个选项按竞赛持久化：

| 选项 | 语义 | 实现位置 |
| --- | --- | --- |
| 拼音提示 | 汉字上方显示拼音 | 渲染时跳过标点/空白 |
| 去除符号 | 隐藏标点与符号，只练文字（Unicode P/S 类） | 文章加载时删除 |
| 符号统一 | 全角/半角标点、全角字母数字视为同一答案 | **只影响判定，不改写原文**（避免破坏 1.5、C++ 等） |
| 去除换行 | 合并段落、去掉首尾缩进；中英混排时保留必要空格 | 文章加载时合并 |

- 存储：`competitions.text_options`（JSON，新增选项不需要改表）；`pinyin_enabled` 继续写入以兼容旧页面。
- 应用顺序：去换行 → 去符号 → 加载为可输入字符数组；`符号统一` 在比较字符时生效（计数、单元格颜色、提交成绩共用同一判定器）。
- 旧数据：`text_options` 为空时由 `pinyin_enabled` 推导；旧客户端只传 `pinyinEnabled` 也能创建。
- 变换只发生在学生端渲染/统计层，**服务端不保存改写后的文章**，成绩口径（正确/已输入）不受影响。

## 8. 构建与发布

- 本地校验：`make test` = 版本门禁 + 前端单测 + `go test ./...` + 前端构建（需先 `npm ci`）。
- 版本：`VERSION` 为唯一来源，必须与 `frontend/package.json`、`package-lock.json` 顶层版本、CHANGELOG 段落一致；打 `vX.Y.Z` tag 时 CI 会校验。
- 发布：push tag → CI 跑测试 → buildx 构建 `linux/amd64,linux/arm64` → 推 `ghcr.io/alumos/keysprint`（`X.Y.Z`/`X.Y`/`X`/`latest`/`sha-*`）→ 附 SBOM/provenance → 自动建 Release。无需自定义 secret。
- 本地镜像：`docker compose up -d --build`（本机 Windows 不装 Docker 时跳过，用第 10 节方式开发）。

## 9. 部署（VPS + EdgeOne CDN）

1. 服务器只装 Docker + Compose，拉 `compose.deploy.yaml` 与 `.env.example` 为 `.env`，把 `JWT_SECRET` 换成 `openssl rand -hex 32`。
2. `docker compose -f compose.deploy.yaml pull && up -d`，访问 `http://服务器:8080`，首次进管理员初始化。
3. EdgeOne：源站填 `http://VPS_IP:8080`，HTTPS 由 CDN 终结，开启 WebSocket（`/ws` 需要透传 Upgrade）。
4. **回源 Host 与 Origin 一致时无需额外配置**；若 CDN 回源 Host 与页面域名不同，给容器加 `ALLOWED_ORIGINS=https://你的加速域名`，否则 WebSocket 会被 403 拒绝。
5. 缓存建议：CDN 遵循源站缓存头即可——`/assets/*` 为一年 immutable（文件名带 hash），其余（含 `index.html`）为 `no-cache`，升级后不会被旧页面卡住。
6. 备份：停止容器后复制 `/data/keysprint.db`（WAL 模式，热拷贝可能不一致）。数据卷默认 `keysprint_data`，本地试跑与生产别共用同名卷。

## 10. Windows 本地开发（不装 Docker）

需要 Go 1.24+ 与 Node.js 22+（本机已用 Go 1.27.3 / Node 24 验证）。

```powershell
# 终端 A：前端 dev server（http://localhost:5173，/api 与 /ws 已代理到 8080）
cd E:\Projects\KeySprint\frontend
npm ci
npm run dev

# 终端 B：后端（数据库自动建在 backend/data/keysprint.db）
cd E:\Projects\KeySprint\backend
$env:JWT_SECRET = "dev-secret-please-change"
$env:ADDR = ":8080"
go run .
```

- 想在 8080 单端口预览：先 `npm run build`，再以 `PUBLIC_DIR=../frontend/dist` 启动后端。
- 重置数据：删除 `backend/data/` 即可（已被 .gitignore 忽略）。
- 常用校验：`go test ./...`（backend 目录）、`node --test tests/*.test.mjs`（frontend 目录）、`npm run build`。
- 国内网络可给 Go 设 `GOPROXY=https://goproxy.cn,direct`、给 npm 设 `--registry=https://registry.npmmirror.com`。

## 11. 运行配置（环境变量）

| 变量 | 默认 | 说明 |
| --- | --- | --- |
| `ADDR` | `:8080` | 容器内监听地址 |
| `DB_PATH` | `/data/keysprint.db`（镜像） | SQLite 文件路径 |
| `PUBLIC_DIR` | `/app/public`（镜像） | 前端静态目录 |
| `TZ` | `Asia/Shanghai` | 时区（二进制内嵌 tzdata） |
| `JWT_SECRET` | 空 → 进程内随机 | 登录签名密钥；生产必须显式设置且持久 |
| `ALLOWED_ORIGINS` | 空 | 允许的 WebSocket 来源，逗号分隔（CDN/回源 Host 不一致时用） |
| `CLASS_SOURCE` | `manual` | 名单来源 manual / classorbit |
| `CLASS_SYSTEM_URL` / `_PATH` / `_TOKEN` / `_TIMEOUT` | 空 / `/api/integration/classes` / 空 / `8s` | ClassOrbit 对接 |
| `HEALTHCHECK_URL` | `http://127.0.0.1:8080/api/health` | `--healthcheck` 子命令目标；改 ADDR 时同步改它 |

`app_meta` 中保存过的 ClassOrbit 配置优先于环境变量。

## 12. 安全模型与已知限制

已加固：WebSocket 必须带凭证且校验 Origin；成绩提交需要比赛编号且停止后只留 3 分钟；首次初始化使用原子插入（防并发抢注）；JWT 不再有公开默认值；导出改为带 `x-admin-token` 的 fetch（token 不进 URL）；公开文章/名单接口需要有效比赛编号；ClassOrbit 请求不跟随重定向；名单与成绩广播只发给教师连接；查询与写操作的数据库错误不再被静默吞掉。

仍然存在的限制（课堂工具的取舍）：
- 学生没有账号，身份自报；拿到比赛编号的人仍可为任意学号提交成绩。若需要强一致，应引入学生侧签名或教师端一次性口令。
- 成绩由客户端上报正确字数/用时，服务端只做范围校验。
- 数据表没有迁移版本号，升级路径依赖幂等迁移；改动 schema 时要按第 4 节两处同时修改。
- 班级分析按班级名聚合，同名班级会合并。
- 导出 CSV 未做公式注入转义（仅教师可导出，风险低）。

## 13. 测试

- 后端：`go test ./...`（初始化/登录/竞赛幂等/成绩派生/ClassOrbit 同步/老库迁移/文本选项/比赛编号校验/宽限期/公开接口门槛/WebSocket 来源与凭证/缓存头）。
- 前端：`node --test tests/*.test.mjs`（打字缓冲区 6 项 + 文本选项 7 项：默认值、label/hint 等长、去符号、去换行、符号等价、旧数据归一、拼音跳过标点）。
- 端到端自检（本地起服务后）：`/api/health` → 初始化 → 登录 → 建文章/竞赛 → 按编号查赛 → 学生 WS 加入 → 教师 WS 收到名单 → 提交成绩 → 结果排名。

## 14. 本次改动：系统配置与分页（v1.7.0）

### 分页
竞赛历史与成绩明细表改为每页 10 条，底部显示「共 N 条 · 第 X / Y 页」并提供上一页/下一页；筛选条件或删除导致条数变化时页码自动收敛，不会出现空页。数据仍在单次请求内取回并在前端切片，服务端接口不变。

### 系统配置（外观）
侧栏新增「系统配置」（仅管理员可见），可修改：

| 字段 | 说明 |
| --- | --- |
| 网站标题 | 侧栏品牌、登录页标题、浏览器标签标题，必填，≤40 字符 |
| 网站副标题 | 侧栏品牌下方小字，可留空，≤60 字符 |
| ICP 备案号 | 页脚显示，留空则不显示；**链接固定为 https://beian.miit.gov.cn/**，前端不允许自定义跳转地址 |

- 存储：`app_meta` 的 `site_title` / `site_tagline` / `icp_number`，无需改表；重启后由 `loadSiteConfig` 读回。
- 接口：`GET /api/site` 公开（登录前就要渲染品牌与页脚），`PUT /api/site` 仅管理员。
- 默认值：标题 `KeySprint`、副标题 `键跃竞赛管理`、备案号**为空**（项目不再内置特定备案号，部署后自行填写）。
- 保存后立即生效：前端通过 Provider 的 `refresh()` 更新侧栏、登录页与页脚，无需刷新页面。

## 15. 上一批改动：v1.6.0（2026-10）


**新增**：竞赛文本选项（四项、JSON 存储、旧数据/旧客户端兼容、共用判定器）；教师 WS 一次性 ticket；`ALLOWED_ORIGINS`；静态资源缓存头。

**修复/加固**：成绩提交需比赛编号 + 停止宽限期；`/ws` 鉴权与来源校验；初始化并发抢注；JWT 默认密钥；`?token=` 泄露；公开文章/名单枚举；ClassOrbit 重定向泄密；`resetPassword` 空指针；写操作错误被吞；`request_id` 空串触发唯一索引冲突导致「第二场竞赛创建失败」；WS 写失败连接残留。

**性能**：赛场输入缓存写入改为 400ms 防抖 + 页面卸载兜底；拼音仅对需要提示的字符计算；WS 广播对慢客户端不阻塞、失败即断开清理；哈希静态资源长缓存。

**兼容性**：旧页面仍可用 `pinyin_enabled` 创建竞赛；但旧标签页的学生端提交、导出链接和 `/ws` 连接需要刷新页面（新协议要求比赛编号/ticket）。
