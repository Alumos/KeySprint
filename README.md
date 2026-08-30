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

默认镜像为 `ghcr.io/alumos/keysprint:latest`，支持 `linux/amd64` 与 `linux/arm64`。生产环境建议在 `.env` 中固定完整版本，例如 `KEYSPRINT_TAG=1.1.0`。数据保存在独立卷 `keysprint_data` 的 `/data/keysprint.db`，更新容器不会删除数据。

### ClassOrbit 8080 + KeySprint 8081

ClassOrbit 已在同一台 VPS 的宿主机 `8080` 端口运行时，直接使用仓库中的 [`compose.vps.yaml`](compose.vps.yaml)：

```bash
mkdir -p /opt/keysprint
cd /opt/keysprint
curl -fsSLO https://raw.githubusercontent.com/Alumos/KeySprint/main/compose.vps.yaml
curl -fsSL https://raw.githubusercontent.com/Alumos/KeySprint/main/.env.vps.example -o .env
```

编辑 `.env`，替换两项密钥：

```dotenv
JWT_SECRET=使用-openssl-rand-hex-32-生成
CLASS_SYSTEM_TOKEN=与-ClassOrbit-完全相同的共享密钥
```

确认 ClassOrbit 自己的部署配置包含相同密钥并监听宿主机 `8080`：

```dotenv
APP_PORT=8080
CLASS_SYSTEM_TOKEN=与-KeySprint-完全相同的共享密钥
```

然后启动 KeySprint：

```bash
docker compose -f compose.vps.yaml pull
docker compose -f compose.vps.yaml up -d
docker compose -f compose.vps.yaml ps
```

访问 `http://VPS_IP:8081`。该 Compose 已加入 Linux Docker 所需的 `host-gateway` 映射，KeySprint 容器会通过 `http://host.docker.internal:8080` 访问 ClassOrbit；不要填写 `http://127.0.0.1:8080`，容器内的回环地址指向 KeySprint 自己。

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

当前稳定版本为 `v1.1.0`。KeySprint 遵循 Semantic Versioning：

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
| `CLASS_SOURCE` | `manual` | 名单主要来源：`manual` 或 `classorbit` |
| `CLASS_SYSTEM_URL` | 空 | ClassOrbit 服务地址 |
| `CLASS_SYSTEM_PATH` | `/api/integration/classes` | 班级同步接口 |
| `CLASS_SYSTEM_TOKEN` | 空 | 与 ClassOrbit 完全一致的共享 Bearer 密钥 |
| `CLASS_SYSTEM_TIMEOUT` | `8s` | 外部请求超时 |

## ✦ ClassOrbit 同步

KeySprint 的班级名单支持两种来源：

- `Excel 手工导入`：默认模式，教师上传 `.xlsx`/`.xls` 名单；
- `ClassOrbit 接口`：教师在班级管理中点击同步，从 ClassOrbit 获取自己管理的班级和学生。

Excel 导入始终保留为课堂兜底。每个班级都会标记实际来源，ClassOrbit 同步只更新或停用接口班级，不会清空手工班级，也不会删除已有比赛成绩。

管理员登录 KeySprint 后，进入 `名单来源配置`，填写：

| 后台字段 | 同机 VPS 配置值 |
| --- | --- |
| 主要来源 | `ClassOrbit 接口` |
| ClassOrbit 服务地址 | `http://host.docker.internal:8080` |
| 班级接口路径 | `/api/integration/classes` |
| 共享 Bearer 密钥 | 与 ClassOrbit 的 `CLASS_SYSTEM_TOKEN` 完全一致 |
| 请求超时 | `8s` |

保存后立即生效，并持久化到 KeySprint 数据库；此后数据库中的可视化配置优先于容器环境变量。接口同步是教师主动触发，不是定时任务。

同步只携带教师用户名与独立 Bearer 密钥，不会转发教师密码或 KeySprint 登录令牌。KeySprint 教师的登录用户名必须与 ClassOrbit 教师用户名一致，否则 ClassOrbit 会返回“教师账号不存在”。班级 ID 应保持稳定；远端改名会更新原班级，远端删除会将对应接口班级停用，已有成绩仍保持正确关联。

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
