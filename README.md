<div align="center">

# ⌨️ KeySprint

### 键跃 · READY // TYPE // SPARK

**面向课堂的中文打字训练与实时竞赛系统**

发令一响，文字在指尖起跑。

[![Verify and publish](https://github.com/Alumos/KeySprint/actions/workflows/publish-container.yml/badge.svg)](https://github.com/Alumos/KeySprint/actions/workflows/publish-container.yml)
![Release](https://img.shields.io/github/v/release/Alumos/KeySprint?style=flat-square)
![Go](https://img.shields.io/badge/Go-1.24-00ADD8?style=flat-square&logo=go&logoColor=white)
![React](https://img.shields.io/badge/React-19-61DAFB?style=flat-square&logo=react&logoColor=20232A)
![Image](https://img.shields.io/badge/image-amd64%20%7C%20arm64-2496ED?style=flat-square&logo=docker&logoColor=white)

</div>

> 「准备——开始！」教师统一发令，学生同步开赛；每一次击键都有反馈，每一段进步都有记录。

KeySprint（键跃）是一套可自托管的中文输入课堂工具。教师选择范文、创建限时竞赛并统一发令，学生通过比赛编号进入赛场，系统实时统计速度、正确率和完成进度，赛后生成个人与班级分析。

## ✦ 核心能力

| 学生赛场 | 教师工作台 | 系统底座 |
| --- | --- | --- |
| 比赛编号快速加入 | 范文上传、编辑与共享 | Go 单进程 + SQLite WAL |
| 名单选人或手动填写身份 | 多场竞赛并行控制 | WebSocket 实时发令 |
| 逐字拼音提示，可按场关闭 | 在线名单与自动停赛 | 刷新恢复与成绩择优保存 |
| 中文输入法组合输入与纠错 | 个人/班级趋势与成绩导出 | ClassOrbit 班级同步 |

```mermaid
flowchart LR
    T[教师选择范文] --> C[创建竞赛 · 分享编号]
    C --> S[学生进入赛场]
    S --> R[统一发令 · 实时输入]
    R --> A[成绩排行 · 趋势分析]
```

## ✦ 服务器部署

服务器只需要 Docker 与 Compose 插件，不需要 Go、Node.js 或项目源码。

GHCR 镜像已经公开，服务器无需登录即可直接拉取。

```bash
mkdir -p /opt/keysprint
cd /opt/keysprint
curl -fsSLO https://raw.githubusercontent.com/Alumos/KeySprint/main/compose.deploy.yaml
curl -fsSL https://raw.githubusercontent.com/Alumos/KeySprint/main/.env.example -o .env
```

编辑 `.env`，至少将 `JWT_SECRET` 换成长随机字符串，可使用 `openssl rand -hex 32` 生成。随后直接拉取成品镜像：

```bash
docker compose -f compose.deploy.yaml pull
docker compose -f compose.deploy.yaml up -d
```

访问 `http://服务器地址:8080`，首次打开会进入管理员初始化页面。

更新与查看日志：

```bash
docker compose -f compose.deploy.yaml pull
docker compose -f compose.deploy.yaml up -d
docker compose -f compose.deploy.yaml logs -f keysprint
```

默认镜像为 `ghcr.io/alumos/keysprint:latest`，支持 `linux/amd64` 与 `linux/arm64`。生产环境建议在 `.env` 中固定完整版本，例如 `KEYSPRINT_TAG=1.0.0`。数据保存在独立卷 `keysprint_data` 的 `/data/keysprint.db`，更新容器不会删除数据。

## ✦ 自动构建

[GitHub Actions](.github/workflows/publish-container.yml) 会执行：

1. 运行 Go 后端测试；
2. 安装锁定的前端依赖并执行 TypeScript/Vite 生产构建；
3. 构建双架构极简镜像；
4. 发布到 `ghcr.io/alumos/keysprint`；
5. 为镜像附加 SBOM 与 provenance；
6. 正式 Tag 构建完成后自动创建 GitHub Release。

推送 `main` 会生成 `latest`、`main` 和 `sha-xxxxxxx`；推送 `v1.2.3` 标签还会生成 `1.2.3`、`1.2` 与 `1`。Pull Request 只测试和构建，不发布镜像。

运行镜像以 `scratch` 为底，只包含静态 Go 二进制、前端资源与 HTTPS 根证书；没有 shell、包管理器或多余系统工具。容器使用非 root 用户、只读根文件系统、应用内健康检查并移除全部 Linux capabilities。

## ✦ 版本与 Tag

当前稳定版本为 `v1.0.0`。KeySprint 遵循 Semantic Versioning：

- `PATCH`：向后兼容的修复与安全更新；
- `MINOR`：向后兼容的新功能；
- `MAJOR`：不兼容的 API、配置或数据行为变化；
- `-rc.N`：正式发布前的候选版本。

根目录 [`VERSION`](VERSION) 是唯一版本源，并与前端包版本、Changelog 和 Git Tag 自动校验。完整规则与发布步骤见 [`RELEASING.md`](RELEASING.md)，版本变化见 [`CHANGELOG.md`](CHANGELOG.md)。已发布 Tag 永不移动或覆盖。

```bash
make version
./scripts/check-version.sh
```

## ✦ 配置

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `KEYSPRINT_IMAGE` | `ghcr.io/alumos/keysprint` | 成品镜像地址 |
| `KEYSPRINT_TAG` | `latest` | 镜像标签，生产环境可固定版本 |
| `DATA_VOLUME_NAME` | `keysprint_data` | SQLite 数据卷名称 |
| `PORT` | `8080` | 对外端口 |
| `TZ` | `Asia/Shanghai` | 服务时区 |
| `JWT_SECRET` | 无 | 必须设置的登录签名密钥 |
| `CLASS_SYSTEM_URL` | 空 | ClassOrbit 服务地址 |
| `CLASS_SYSTEM_PATH` | `/api/integration/classes` | 班级同步接口 |
| `CLASS_SYSTEM_TOKEN` | 空 | 可选的共享 Bearer 密钥 |
| `CLASS_SYSTEM_TIMEOUT` | `8s` | 外部请求超时 |

## ✦ ClassOrbit 同步

KeySprint 可以从 [ClassOrbit](https://github.com/Alumos/ClassOrbit) 获取当前教师管理的班级和学生名单。管理员在“积分系统配置”中填写地址和共享密钥后即可同步。

同步只携带教师用户名与独立 Bearer 密钥，不会转发教师密码或 KeySprint 登录令牌。班级 ID 应保持稳定；远端改名会更新原班级，删除会将班级停用，已有成绩仍保持正确关联。

## ✦ 本地开发

需要 Go 1.24+ 与 Node.js 22+。

```bash
cd frontend
npm ci
npm run dev
```

另开终端：

```bash
cd backend
ADDR=:8080 DB_PATH=./data/keysprint.db PUBLIC_DIR=../frontend/dist go run .
```

完整验证：

```bash
make test
```

如果需要从源码构建本地镜像：

```bash
cp .env.example .env
docker compose up -d --build
```

## ✦ 备份

SQLite 使用 WAL 模式。停止服务后复制数据库可获得一致备份：

```bash
docker compose -f compose.deploy.yaml stop keysprint
docker compose -f compose.deploy.yaml cp keysprint:/data/keysprint.db ./keysprint-backup.db
docker compose -f compose.deploy.yaml start keysprint
```

生产环境请在 KeySprint 前配置 HTTPS 反向代理，并妥善保管 `.env`、数据卷与备份。
