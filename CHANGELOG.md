# Changelog

本项目遵循 CalVer：`YYYY.MM.DD.MICRO`，标题格式 `## [YYYY.MM.DD.MICRO] - SemVer`。

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
