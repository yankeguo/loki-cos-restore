# loki-cos-restore

一个用于从腾讯云 COS 存储桶中恢复 Loki 归档日志的工具。

## 功能概述

`loki-cos-restore` 是一个专门设计用于恢复存储在腾讯云 COS (Cloud Object Storage) 中的 Loki 日志数据的工具。它能够：

- 根据指定的时间范围和查询条件恢复归档的日志数据
- 支持批量恢复操作
- 集成 Loki 的配置系统，支持标准的 Loki 命令行参数
- 支持不同的恢复优先级（Standard/Bulk）

## 环境变量

工具通过以下环境变量进行配置：

| 环境变量           | 描述                              | 默认值     | 必需 |
| ------------------ | --------------------------------- | ---------- | ---- |
| `RESTORE_USER_ID`  | 用户 ID，支持逗号分隔的多个值     | `fake`     | 否   |
| `RESTORE_TIME_BEG` | 开始时间 (RFC3339 格式)           | -          | 是   |
| `RESTORE_TIME_END` | 结束时间 (RFC3339 格式)           | -          | 是   |
| `RESTORE_QUERY`    | 日志查询表达式                    | -          | 是   |
| `RESTORE_DAYS`     | 恢复天数                          | `3`        | 否   |
| `RESTORE_TIER`     | 恢复优先级 (`Standard` 或 `Bulk`) | `Standard` | 否   |

## Loki 命令行参数

除了环境变量外，工具还支持 Loki 标准的命令行参数：

- `-config.file=/etc/loki/config/config.yaml` - 指定 Loki 配置文件路径
- `-config.expand-env=true` - 启用配置文件中的环境变量扩展

### 示例命令

```bash
# 基本使用
RESTORE_TIME_BEG="2024-01-01T00:00:00Z" \
RESTORE_TIME_END="2024-01-02T00:00:00Z" \
RESTORE_QUERY='{app="nginx"}' \
./loki-cos-restore -config.file=/etc/loki/config.yaml -config.expand-env=true

# 使用自定义恢复天数和优先级
RESTORE_TIME_BEG="2024-01-01T00:00:00Z" \
RESTORE_TIME_END="2024-01-02T00:00:00Z" \
RESTORE_QUERY='{app="nginx"}' \
RESTORE_DAYS="7" \
RESTORE_TIER="Bulk" \
./loki-cos-restore -config.file=/etc/loki/config.yaml -config.expand-env=true
```

## 配置要求

工具需要访问 Loki 的存储配置，特别是 AWS S3 兼容的存储配置（用于 COS 访问）。确保在 Loki 配置文件中包含：

```yaml
storage_config:
  aws:
    s3: s3://access_key:secret_key@region/bucket_name
    endpoint: cos.<region>.myqcloud.com
    bucketnames: your-bucket-name
    access_key_id: your-access-key
    secret_access_key: your-secret-key
```

## 时间格式

时间参数必须使用 RFC3339 格式：

- `2024-01-01T00:00:00Z`
- `2024-01-01T08:00:00+08:00`

## 查询语法

支持标准的 Loki LogQL 查询语法：

- `{label="value"}` - 标签匹配
- `{app="nginx",level="error"}` - 多标签匹配
- `{app=~"nginx|apache"}` - 正则表达式匹配

## 恢复优先级

- `Standard` - 标准优先级，恢复时间较短
- `Bulk` - 批量优先级，恢复时间较长但成本较低

## 构建和部署

### 本地构建

```bash
go build -o loki-cos-restore .
```

### Docker 构建

```bash
# 使用提供的构建脚本
./dev.sh

# 或手动构建
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o loki-cos-restore .
docker build -t loki-cos-restore:latest .
```

## 日志输出

工具会输出详细的执行日志，包括：

- 配置参数确认
- Loki 实例初始化状态
- 查询匹配结果数量
- 每个文件的恢复状态

## 错误处理

- 缺少必需的环境变量会报错并退出
- 时间格式错误会报错并退出
- 查询语法错误会报错并退出
- 网络或权限问题会在日志中显示具体错误信息

## 注意事项

1. 确保有足够的 COS 权限进行恢复操作
2. 大批量恢复操作可能需要较长时间
3. 恢复操作会产生相应的 COS 费用
4. 建议在非高峰期进行批量恢复操作

## 许可证

[LICENSE](LICENSE) 文件中查看详细信息。
