# Changelog

本项目遵循 CalVer：`YYYY.MM.DD.MICRO`，标题格式 `## [YYYY.MM.DD.MICRO] - SemVer`。

## [2026.09.20.0] - 0.1.2

### 🐛 Bug Fixes  问题修复

- 【稀疏定位】修复 timeslice 调用不存在的 `timefmt.ParseLineValue` 导致全项目不可编译（改用 `ParseWith`），并清理未使用变量与冗余形参 `layoutStr`
- 【稀疏定位】回退语义改为 `Span.Reason` 承载说明、`error` 返回 nil，避免调用方因判错而丢失 FullScan 指引

### 📚 Docs 文档更新

- 新增 `docs/04-项目质量审查报告.md`，记录 P0 修复前后的构建/静态检查/测试实测数据

## [2026.08.04.1] - 0.1.1

### ✨ New Features 新增功能

- 【配置安全】ENC() AES-256-GCM 加解密、密钥文件 0600、密钥失败即终止
- 【安全】输出/日志脱敏：IPv4/手机号/身份证/邮箱
- 【日志】按 AGENTS 规范级别/格式/按日轮转/保留 32 天
- 【CLI】相对时间 `--since`、读限速 `--read-rate`、JSON 摘要 `--json`、退出码分类
- 【测试】配置/加密/脱敏/日志/限速/相对时间/CLI 端到端

## [2026.08.04.0] - 0.1.0

### ✨ New Features 新增功能

- 【核心】预检探测：头尾 `ReadAt` 采样，识别编码与时间格式
- 【核心】时间切片：闭开窗口 `[start, end)` 过滤与坏行计数
- 【核心】内容检索：字节扫描 / AC-FSM 多词 / RE2 兜底
- 【核心】流式导出：只读分块扫描、唯一写出口、中文执行摘要
- 【CLI】`inspect` / `slice` / `grep` / `export` 四命令
- 【资源】默认 GOMEMLIMIT≈192MiB、单行 1MiB、块 1MiB、顺序执行

### 📚 Docs 文档更新

- 新增 `docs/01` PRD、`docs/02` 架构、`docs/03` 实施计划（含资源红线）

