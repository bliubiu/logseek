# logSeek

面向运维的**大文本日志命令行解析工具**。全程**只读**源日志文件，只写输出结果文件。

## 特性

| 能力 | 说明 |
|---|---|
| 头尾预检 | `ReadAt` 随机采样首尾，识别编码、换行风格、时间格式与首末时间 |
| 时间切片 | 闭开窗口 `[start, end)` 过滤，支持绝对时间与相对时间（`--since 30m`） |
| 内容检索 | 字节级多关键词扫描（OR/AND）、RE2 正则兜底、整词与大小写选项 |
| 流式导出 | 单趟顺序读 + 块缓冲，默认软内存上限约 192MiB，支持读限速 |
| 结果脱敏 | 控制台摘要、日志文件、导出结果统一脱敏（IPv4/手机号/身份证/邮箱） |
| 配置安全 | 口令以 `ENC()` AES-256-GCM 加密存储，密钥文件 0600，失败即终止不降级 |

设计上遵循 DDD 分层（`cmd → application → domain`，`infrastructure` 通过端口注入），
依赖仅 `cobra` 一个第三方库。

## 安装与构建

```bash
go build -o bin/logseek.exe ./cmd/logseek   # Windows
go build -o bin/logseek ./cmd/logseek       # Linux/macOS
```

也可直接运行：`go run ./cmd/logseek <命令> [参数]`。

## 命令

### inspect — 头尾预检

```bash
logseek inspect app.log
logseek inspect app.log --json
```

### slice — 时间窗口切片

```bash
logseek slice app.log --start "2026-01-01 00:00:00" --end "2026-01-02 00:00:00" -o out.log
logseek slice app.log --since 30m -o out.log          # 最近 30 分钟
logseek slice app.log --since 2h --time-format "2006-01-02 15:04:05"
```

### grep — 内容检索

```bash
logseek grep app.log -k ERROR -k WARN
logseek grep app.log -k ORA- -k "00600" --and
logseek grep app.log --pattern 'ORA-\d{5}'
```

### export — 时间 ∧ 内容组合

```bash
logseek export app.log --since 7d -k ORA- -o report.log
```

## 通用参数

| 参数 | 默认值 | 说明 |
|---|---|---|
| `--start` / `--end` | — | 绝对时间窗口，`--end` 不含；与 `--since` 互斥 |
| `--since` | — | 相对时间窗口，支持 `30s/15m/2h/7d` |
| `--time-format` | 自动探测 | Go 时间布局，探测失败时使用 |
| `-k, --keyword` | — | 关键词，可多次指定 |
| `--and` | false | 多关键词按「与」组合，默认「或」 |
| `--ignore-case` | true | 忽略大小写 |
| `--word-regexp` | false | 整词匹配 |
| `--pattern` | — | RE2 正则（复杂模式兜底） |
| `-o, --output` | — | 结果文件路径，省略则只输出摘要 |
| `--read-rate` | 0（不限） | 读限速字节/秒，生产建议 `67108864`（64MiB/s） |
| `--mem-limit-mib` | 192 | Go 软内存上限，不得设为 0 关闭 |
| `--mask` | true | 控制台/摘要/导出脱敏 |
| `--json` | false | 以 JSON 输出摘要与预检报告 |
| `--log-level` | INFO | DEBUG / INFO / ERROR |
| `--log-dir` | logs | 日志目录，按日轮转保留 32 天 |
| `--config` | — | 配置文件路径（JSON） |
| `--key-file` | `.logseek/key` | 密钥文件路径 |
| `--save-config` | — | 命令成功后把当前配置落盘（口令加密保存） |
| `--ensure-password` | false | 配合 `--save-config`：空口令自动生成强口令 |

## 退出码

脚本可直接消费：

| 退出码 | 含义 |
|---|---|
| 0 | 成功 |
| 1 | 运行时错误 |
| 2 | 命令行用法错误 |
| 3 | 源文件或资源不存在 / 无读取权限 |
| 4 | 预检失败（无法识别时间格式、文件为空等） |
| 130 | 被中断 |

退出码由错误类型（`internal/domain/errkind`）决定，不依赖错误文本。

## 配置文件

```json
{
  "log_level": "INFO",
  "log_dir": "logs",
  "key_file": ".logseek/key",
  "password": "ENC(...)",
  "default_block_size": 1048576,
  "enable_mask": true
}
```

口令请始终以 `ENC(...)` 密文保存。明文口令不会被阻断，但会给出安全提示；
执行 `logseek inspect x.log --save-config app.json --ensure-password` 可生成强口令并加密落盘。

## 开发

```bash
go build ./...                                    # 构建
go vet ./...                                      # 静态检查
go test ./... -count=1                            # 全量测试
go test ./... -count=1 -coverprofile=cov.out       # 覆盖率
go tool cover -func=cov.out | tail -1
```

更多文档见 `docs/`：`00-安全设计`、`01-大文件日志解析工具PRD`、
`02-架构设计方案`、`03-实施计划`、`04-项目质量审查报告`。
