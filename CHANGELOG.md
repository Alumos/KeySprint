# Changelog

KeySprint 遵循 [Semantic Versioning](https://semver.org/)，所有面向用户的重要变化都记录在此文件中。

## [Unreleased]

### Planned

- 后续变更先进入本节，发布时再归入对应版本。

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

[Unreleased]: https://github.com/Alumos/KeySprint/compare/v1.0.0...HEAD
[1.0.0]: https://github.com/Alumos/KeySprint/releases/tag/v1.0.0
