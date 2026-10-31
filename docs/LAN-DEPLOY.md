# Windows 局域网部署（免 Docker）

> 面向没有云服务器、不能装 Docker、机器是 Windows 的教师：把教师机变成课堂服务器，学生在同一局域网用浏览器访问。
> 运行时**不需要外网**，因此适合只能访问国内网络的教学专网。内网访问不需要备案号。

## 1. 架构

```
学生机 / 教室大屏（Chrome、Edge）
        │  http://<教师机IPv4>:8080
        ▼
教师 Windows 电脑
  ├─ keysprint.exe      单个 Go 二进制（含 WebSocket、SQLite、静态站点）
  ├─ web\               前端静态文件（全部本地，无 CDN/字体外链）
  ├─ data\keysprint.db  SQLite 数据（WAL）
  └─ config.env         端口、JWT_SECRET 等配置（首次启动自动生成）
```

一个 exe 同时提供页面、JSON API 与 WebSocket。没有 Redis、没有独立数据库、不需要域名或 HTTPS。

## 2. 获取安装包

两种来源，任选其一：

| 来源 | 说明 |
| --- | --- |
| GitHub Release 附件 | 下载 `keysprint-<版本>-windows-x64.zip`；教育专网访问 GitHub 可能受限，可用 U 盘/国内网盘转存 |
| Actions artifact | 每次 push 都会构建，名 `keysprint-windows-x64`；登录 GitHub → Actions → 对应运行 → Artifacts 下载 |

包内容（约 10–12 MB）：

```
KeySprint-Windows-x64\
  keysprint.exe            服务主程序（amd64，纯静态，无需 .NET/VC++ 运行库）
  web\                     前端页面
  data\                    数据库目录（首次启动生成）
  logs\                    运行日志（首次启动创建）
  config.env               首次启动自动生成
  config.env.example       配置说明示例
  start.bat / stop.bat     启动 / 停止
  allow-firewall.bat       放行 8080（管理员）
  install-autostart.bat    注册开机自启（管理员）
  remove-autostart.bat     取消开机自启（管理员）
  README.txt               中文使用说明
  VERSION.txt              版本与构建信息
```

## 3. 首次部署（约 5 分钟）

1. **完整解压**到固定目录，例如 `D:\KeySprint`（不要在压缩包内直接双击 exe，缺少 `web` 会启动失败）。
2. 右键 `allow-firewall.bat` → **以管理员身份运行**，放行 TCP 8080。
3. 双击 `start.bat`：首次自动生成 `config.env`（含随机 `JWT_SECRET`）与 `data` 目录，窗口保持打开即服务运行中。
4. 本机浏览器打开 `http://127.0.0.1:8080`，完成管理员初始化（创建账号密码）。
5. 打开 `cmd` 运行 `ipconfig`，记下「IPv4 地址」（例如 `192.168.1.23`）。
   - **强烈建议把教师机 IP 固定**（静态 IP 或路由器 DHCP 保留），否则地址变化学生就连不上。
6. 学生电脑浏览器访问 `http://192.168.1.23:8080`，输入老师给的比赛编号即可；学生无需账号、无需安装任何软件。
7. 可选：管理员身份运行 `install-autostart.bat`，Windows 启动后自动运行，不需要登录桌面。

教师机网络位置建议设为「专用网络」（否则 Windows 防火墙默认拦截入站）；首次启动若弹出防火墙提示，勾选允许「专用网络」。

## 4. 日常使用

| 场景 | 操作 |
| --- | --- |
| 开始上课 | 确认服务在运行（浏览器打开 `http://127.0.0.1:8080` 能进后台），提前创建竞赛并分享编号 |
| 停止服务 | 双击 `stop.bat`，或直接关闭启动窗口 |
| 学生连不上 | 检查教师机 IP 是否变化、是否放行 8080、两台机器是否同网段 |
| 修改端口 | 编辑 `config.env` 的 `ADDR=:8081`，同步改 `allow-firewall.bat` 里的 `PORT` 并重新放行 |
| 修改站点外观 | 后台「系统配置」改标题/副标题；内网不需要备案号，留空即可 |
| 多个老师共用 | 管理员在「教师管理」新建账号；每位老师的文章/班级/竞赛/成绩互相独立 |

注意：教师机不要进入睡眠/休眠（电源计划设为高性能、笔记本插电），否则服务会中断、学生掉线。

## 5. 备份、升级与迁移

- **备份**：先停止服务，再复制整个 `data` 目录（含 `keysprint.db`、`-wal`、`-shm`）；建议每周复制到 U 盘或学校网盘。
- **升级**：停止服务 → 用新包覆盖 `keysprint.exe` 与 `web` → 保留 `data` 与 `config.env` → 启动。数据库结构在启动时自动迁移。
- **换机器**：整个目录拷走即可（数据全在 `data` 内）。

## 6. 多教师 / 多机房的组织方式

| 方式 | 说明 |
| --- | --- |
| 每台教师机各自一套（最省事） | 各老师解压到自己的电脑，数据各自独立；地址随 IP 变，适合单人使用 |
| 学校中心机（推荐常态化） | 找一台常开的 Windows 或 Linux 机器部署一套，多个教师账号共用；地址固定、便于备份 |
| 机房专用服务器 | 若机房有 Linux 服务器，也可用容器方式部署（见 README 的服务器部署），但这需要 IT 支持 |

## 7. 常见问题

| 现象 | 原因与处理 |
| --- | --- |
| 双击 `start.bat` 闪退 | 查看 `logs\keysprint.log`；常见原因是未完整解压或端口被占用 |
| 学生打不开页面 | 教师机 IP 变了 / 防火墙未放行 / 不同网段；先在教师机浏览器试 `http://127.0.0.1:8080` |
| 页面能开但比赛连不上 | WebSocket 被防火墙拦截，重新以管理员运行 `allow-firewall.bat` |
| 复制比赛编号失败 | 已内置兼容方案（非安全上下文退回 `execCommand`）；仍失败时手动选中复制 |
| 端口被占用 | 改 `config.env` 的 `ADDR`，同步改防火墙端口 |
| 杀毒软件拦截 exe | 把整个目录加入白名单 |
| 开机自启没生效 | 管理员运行 `install-autostart.bat`；在「任务计划程序」确认 `KeySprint` 任务存在并已启用 |
| 教师重启后要重新登录 | `config.env` 被删除或 `JWT_SECRET` 被改动；恢复该文件即可 |

## 8. 与容器部署的差异

| 项 | Windows 便携包 | Docker 镜像 |
| --- | --- | --- |
| 依赖 | 无（纯静态 exe） | Docker + Compose |
| 升级 | 替换 exe 与 web | `docker compose pull && up -d` |
| 数据 | 包内 `data` 目录 | named volume `/data` |
| 适用于 | 机房/教室个人电脑 | VPS、学校服务器、需要 HTTPS/CDN 的场景 |
| 功能 | 完全一致 | 完全一致 |
