# loki-cos-restore

A tool to restore archived objects from COS bucket for Loki.

## Overview

`loki-cos-restore` is a specialized tool designed to restore Loki log data stored in Tencent Cloud COS (Cloud Object Storage). It can:

- Restore archived log data based on specified time ranges and query conditions
- Support batch restore operations
- Integrate with Loki's configuration system, supporting standard Loki command-line arguments
- Support different restore priorities (Standard/Bulk)

## Environment Variables

The tool is configured through the following environment variables:

| Variable           | Description                                          | Default    | Required |
| ------------------ | ---------------------------------------------------- | ---------- | -------- |
| `RESTORE_USER_ID`  | User ID(s), supports comma-separated multiple values | `fake`     | No       |
| `RESTORE_TIME_BEG` | Start time (RFC3339 format)                          | -          | Yes      |
| `RESTORE_TIME_END` | End time (RFC3339 format)                            | -          | Yes      |
| `RESTORE_QUERY`    | Log query expression                                 | -          | Yes      |
| `RESTORE_DAYS`     | Restore days                                         | `3`        | No       |
| `RESTORE_TIER`     | Restore priority (`Standard` or `Bulk`)              | `Standard` | No       |

## Loki Command Line Arguments

In addition to environment variables, the tool supports Loki standard command-line arguments:

- `-config.file=/etc/loki/config/config.yaml` - Specify Loki configuration file path
- `-config.expand-env=true` - Enable environment variable expansion in configuration files

### Example Commands

```bash
# Basic usage
RESTORE_TIME_BEG="2024-01-01T00:00:00Z" \
RESTORE_TIME_END="2024-01-02T00:00:00Z" \
RESTORE_QUERY='{app="nginx"}' \
./loki-cos-restore -config.file=/etc/loki/config.yaml -config.expand-env=true

# Using custom restore days and priority
RESTORE_TIME_BEG="2024-01-01T00:00:00Z" \
RESTORE_TIME_END="2024-01-02T00:00:00Z" \
RESTORE_QUERY='{app="nginx"}' \
RESTORE_DAYS="7" \
RESTORE_TIER="Bulk" \
./loki-cos-restore -config.file=/etc/loki/config.yaml -config.expand-env=true
```

## Configuration Requirements

The tool requires access to Loki's storage configuration, particularly AWS S3-compatible storage configuration (for COS access). Ensure your Loki configuration file includes:

```yaml
storage_config:
  aws:
    s3: s3://access_key:secret_key@region/bucket_name
    endpoint: cos.<region>.myqcloud.com
    bucketnames: your-bucket-name
    access_key_id: your-access-key
    secret_access_key: your-secret-key
```

## Time Format

Time parameters must be in RFC3339 format:

- `2024-01-01T00:00:00Z`
- `2024-01-01T08:00:00+08:00`

## Query Syntax

Supports standard Loki LogQL query syntax:

- `{label="value"}` - Label matching
- `{app="nginx",level="error"}` - Multi-label matching
- `{app=~"nginx|apache"}` - Regular expression matching

## Restore Priority

- `Standard` - Standard priority, shorter restore time
- `Bulk` - Bulk priority, longer restore time but lower cost

## Build and Deployment

### Local Build

```bash
go build -o loki-cos-restore .
```

### Docker Build

```bash
# Using the provided build script
./dev.sh

# Or manual build
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o loki-cos-restore .
docker build -t loki-cos-restore:latest .
```

## Log Output

The tool outputs detailed execution logs, including:

- Configuration parameter confirmation
- Loki instance initialization status
- Query match results count
- Restore status for each file

## Error Handling

- Missing required environment variables will error and exit
- Time format errors will error and exit
- Query syntax errors will error and exit
- Network or permission issues will show specific error information in logs

## Notes

1. Ensure sufficient COS permissions for restore operations
2. Large batch restore operations may take considerable time
3. Restore operations will incur corresponding COS charges
4. Recommend performing batch restore operations during off-peak hours

## License

See [LICENSE](LICENSE) file for details.

## Related Documentation

- [Chinese README](README.zh.md) - 中文文档
