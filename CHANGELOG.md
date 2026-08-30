# Changelog

KeySprint 遵循 [Semantic Versioning](https://semver.org/)，所有面向用户的重要变化都记录在此文件中。

## [Unreleased]

### Planned

- 后续变更先进入本节，发布时再归入对应版本。

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

[Unreleased]: https://github.com/Alumos/KeySprint/compare/v1.1.0...HEAD
[1.1.0]: https://github.com/Alumos/KeySprint/releases/tag/v1.1.0
[1.0.0]: https://github.com/Alumos/KeySprint/releases/tag/v1.0.0
