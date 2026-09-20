# Changelog

本项目遵循 CalVer：`YYYY.MM.DD.MICRO`，标题格式 `## [YYYY.MM.DD.MICRO] - SemVer`。

## [2026.09.20.2] - 0.2.1

详见 `docs/05-大文件端到端与性能测试报告.md`（基于 testdata/alert_dlscdb1.log，10.84GB / 1.8 亿行实跑）。

### 🐛 Bug Fixes  问题修复

- 【预检】修复尾部样例展示的不是文件真实末尾：`tailLines` 原实现取尾部窗口「前 15 行中的后 5 行」，改为跳过窗口起始处的半行残片后取末尾 5 行
- 【预检】修复多行日志（如 Oracle alert）末条时间恒为空：原仅对样例最后一行尝试解析，改为在尾部窗口内逆向按行查找，窗口内无结果时再向前扩展至 8 个窗口
- 【预检】首条时间同步改为在头部窗口内正向查找，不再局限于样例行
- 【测试】新增 3 条预检回归用例，均已验证对旧实现 FAIL、对新实现 PASS；probe 包覆盖率 72.8% → 74.6%

### 📚 Docs 文档更新

- 新增 `docs/05-大文件端到端与性能测试报告.md`：10GB 样本的端到端验证、吞吐/内存/写盘三维性能测试、缺陷清单与修复建议

## [2026.09.20.1] - 0.2.0

本次为 `docs/04-项目质量审查报告` 中 P1/P2/P3 问题的分批整治，含破坏性 API 变更。

### ⚠️ Breaking Changes  破坏性变更

- 【稀疏定位】`timeslice.Locate` 形参移除冗余的 `layoutStr`（与 `layout.Layout` 完全重复）；`FullScan` 回退不再返回 error，原因改由 `Span.Reason` 承载
- 【脱敏】删除 `mask.MaskWriter`：块级脱敏会切开跨写边界的行导致半个 IP 漏脱敏，改为 `FileSink` 按行施加脱敏钩子
- 【流式】`stream.Options` 移除默认资源档位自动填充的外部依赖，改由调用方注入 `Opener`/`RateLimiter` 端口
- 【退出码】退出码判定由中文错误文本子串匹配改为错误类型判定，新增 `internal/domain/errkind` 包

### ✨ New Features 新增功能

- 【配置安全】新增 `--save-config` / `--ensure-password`：命令成功后经 `PersistentPostRun` 落盘配置，口令统一加密为 `ENC(...)` 保存（文件 0600），缺密钥句柄时拒绝保存避免明文落盘
- 【配置安全】`crypto.RandomPassword` 提供 `crypto/rand` 强口令（拒绝采样去偏、排除易混淆与需转义字符），支撑空口令自动生成
- 【配置安全】加载到明文口令不再静默放行，写入 `Config.Warnings` 并由 CLI 回显安全提示
- 【导出脱敏】导出结果文件接入行级脱敏（`sink.NewWithOptions` / `Options.Mask`），终结此前导出明文落盘的状态
- 【工程】新增 `README.md`（安装、四命令示例、参数表、退出码表、配置示例）
- 【工程】新增 GitHub Actions 质量门禁（build / vet / gofmt / test / race / 覆盖率门禁 65%）

### 🐛 Bug Fixes  问题修复

- 【脱敏】IPv4 规则增加三类误伤豁免：五段及以上版本号（如 `11.2.0.4.0`）、版本语境词前缀（`v`/`Version`/`Release`）、非法段值（越界与前导零）
- 【资源】`stream.Run` 改为具名返回值并在所有返回路径 `defer` 关闭 sink，修复错误路径句柄泄漏与缓冲丢失
- 【资源】新增 `exitNow` 统一退出入口：`RunE` 内直接 `os.Exit` 会跳过 `PersistentPostRun`，导致日志缓冲丢失、句柄不释放
- 【输出】`FileSink.Close` 改为幂等；丢弃型 sink 修复不计数缺陷
- 【仓库】`.gitignore` 精确屏蔽 `testdata/alert_dlscdb1.log`(10GB) 与 `*.gz`(294MB)，保留 CLI 端到端依赖的小样本入库

### 📈 Improvements 性能/体验优化

- 【架构】解除 `domain/stream` 对 `infrastructure/ratelimit`、`resources` 的反向依赖，domain 层不再直接 `os.Open`；改由 application 装配 `fileio.NewOpener` 与限速实现注入
- 【清理】删除 `main.go` 中压编译器警告的假引用 `var _ = mask.Apply` 与 `var _ = io.EOF`
- 【清理】探针脚本 `probe_density.go` 移入 `tools/` 并加 `//go:build probe`，避免每次 `go build` 多编译一个 main 包

### 📚 Docs 文档更新

- 同步 `docs/04-项目质量审查报告.md` 的「整改进展」段落，标注各批次对应提交

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

