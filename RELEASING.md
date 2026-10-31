# 版本与发布规范

## 版本规则

KeySprint 使用 Semantic Versioning，唯一版本源为根目录 [`VERSION`](VERSION)。

| 变化 | 版本升级 | 示例 |
| --- | --- | --- |
| 向后兼容的修复、安全更新或小幅优化 | PATCH | `1.0.0 → 1.0.1` |
| 向后兼容的新功能或明显能力扩展 | MINOR | `1.0.1 → 1.1.0` |
| 不兼容的 API、配置或数据行为变化 | MAJOR | `1.1.0 → 2.0.0` |
| 发布前验证版本 | Pre-release | `1.1.0-rc.1` |

正式 Git Tag 必须为 `vMAJOR.MINOR.PATCH`，候选版本使用 `vMAJOR.MINOR.PATCH-rc.N`。Tag 一旦发布便不可移动、覆盖或重复使用。

## 分支与镜像标签

| Git 事件 | GHCR 标签 | GitHub Release |
| --- | --- | --- |
| 推送 `main` | `latest`、`main`、`sha-xxxxxxx` | 不创建 |
| Tag `v1.2.3` | `v1.2.3`、`1.2.3`、`1.2`、`1`、`sha-xxxxxxx` | 正式 Release（附 Windows 便携包） |
| Tag `v1.3.0-rc.1` | `v1.3.0-rc.1`、`1.3.0-rc.1`、`sha-xxxxxxx` | Pre-release |
| Pull Request | 只测试与构建 | 不创建 |

生产部署建议固定完整版本标签，例如 `KEYSPRINT_TAG=1.2.3`；希望自动跟随最新稳定构建时再使用 `latest`。

## 发布步骤

1. 将 `CHANGELOG.md` 的 Unreleased 内容归入新版本，并写入发布日期。
2. 同步更新 `VERSION`、`frontend/package.json` 与 `frontend/package-lock.json`。
3. 运行 `make test`，版本检查会验证 SemVer、三个版本字段与 Changelog。
4. 提交发布变更，例如 `release: v1.2.3`。
5. 在该提交创建带说明的 Tag 并推送：

   ```bash
   git tag -a v1.2.3 -m "KeySprint v1.2.3"
   git push origin main
   git push origin v1.2.3
   ```

6. 等待 GitHub Actions 完成测试、`linux/amd64` 镜像发布、Windows 便携包构建和 GitHub Release 创建。
7. 匿名读取 GHCR 清单或在服务器执行 `docker compose pull`，确认版本可用。

## 产物

| 产物 | 位置 | 说明 |
| --- | --- | --- |
| 容器镜像 | `ghcr.io/alumos/keysprint` | `linux/amd64`；Tag 生成 `X.Y.Z`/`X.Y`/`X`，main 生成 `latest`/`main`/`sha-*` |
| Windows 便携包 | GitHub Release 附件 + Actions artifact | `keysprint-<版本>-windows-x64.zip`，解压即用，不需要 Docker 与 .NET/VC++ 运行库 |

Windows 包由 `scripts/package-windows.ps1` 在 `windows-latest` 上原生构建（GOARCH=amd64，CGO 关闭），内容为 `keysprint.exe`、`web/` 前端静态文件、启动/防火墙/开机自启脚本与中文部署说明；`data/` 与 `config.env` 在首次运行时生成。

## 自动约束

[`scripts/check-version.sh`](scripts/check-version.sh) 会阻止以下情况进入发布流水线：

- `VERSION` 不是有效 SemVer；
- 前端包或锁文件版本与 `VERSION` 不一致；
- Changelog 缺少当前正式版本；
- Git Tag 与 `VERSION` 不一致。

镜像构建会把版本号和完整提交哈希写入二进制。部署后可以使用：

```bash
docker run --rm ghcr.io/alumos/keysprint:1.2.0 --version
curl http://127.0.0.1:8080/api/health
```
