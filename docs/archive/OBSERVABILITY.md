# Observability Guide

This guide explains how to use the observability features in code-reviewer to monitor API calls, track performance, and debug issues.

## Overview

Code-reviewer includes built-in observability for all LLM provider interactions:

- **Logging**: Request/response logging with API key redaction
- **Metrics**: Duration, token usage, and error tracking
- **Cost Tracking**: Automatic cost calculation per provider

## Configuration

### Default Settings

By default, observability is enabled with these settings:

```yaml
observability:
  logging:
    enabled: true
    level: info              # Options: debug, info, error
    format: human            # Options: json, human
    redactAPIKeys: true      # Always redact API keys in logs
  metrics:
    enabled: true
```

### YAML Configuration

Create or update your `bop.yaml` file:

```yaml
observability:
  logging:
    enabled: true
    level: debug             # More verbose output
    format: json             # Machine-readable logs
    redactAPIKeys: true      # Recommended: keep this enabled
  metrics:
    enabled: true
```

### Environment Variables

Override settings using environment variables:

```bash
# Disable logging
export CR_OBSERVABILITY_LOGGING_ENABLED=false

# Set log level to debug
export CR_OBSERVABILITY_LOGGING_LEVEL=debug

# Use JSON format
export CR_OBSERVABILITY_LOGGING_FORMAT=json

# Disable API key redaction (NOT recommended)
export CR_OBSERVABILITY_LOGGING_REDACT_API_KEYS=false

# Disable metrics
export CR_OBSERVABILITY_METRICS_ENABLED=false
```

## Logging

### Log Levels

- **debug**: All requests, responses, and errors (verbose)
- **info**: Successful requests and responses (default)
- **error**: Only errors and failures

### Log Formats

#### Human Format (Default)

Easy to read for manual inspection:

```
[2025-10-21 12:34:56] REQUEST  provider=openai model=gpt-4o chars=1523 apiKey=****3f2a
[2025-10-21 12:34:58] RESPONSE provider=openai model=gpt-4o duration=2.3s tokens=1234/567 cost=$0.0234
```

#### JSON Format

Machine-readable for log aggregation tools:

```json
{
  "timestamp": "2025-10-21T12:34:56Z",
  "level": "info",
  "type": "request",
  "provider": "openai",
  "model": "gpt-4o",
  "promptChars": 1523,
  "apiKey": "****3f2a"
}
{
  "timestamp": "2025-10-21T12:34:58Z",
  "level": "info",
  "type": "response",
  "provider": "openai",
  "model": "gpt-4o",
  "duration": "2.3s",
  "tokensIn": 1234,
  "tokensOut": 567,
  "cost": 0.0234,
  "statusCode": 200,
  "finishReason": "stop"
}
```

### API Key Redaction

For security, API keys are automatically redacted in logs, showing only the last 4 characters:

```
apiKey: ****3f2a  (original: sk-proj-abc123...3f2a)
```

**Security Note**: Keep `redactAPIKeys: true` enabled to prevent accidental exposure of credentials in logs.

## Metrics

When metrics are enabled, code-reviewer tracks:

### Request Metrics
- Total requests per provider/model
- Request rate over time
- Error counts by type (auth, rate limit, timeout, etc.)

### Performance Metrics
- Request duration (min/max/avg)
- Time spent per provider
- API response times

### Token Metrics
- Input tokens (prompt size)
- Output tokens (completion size)
- Total tokens per provider/model

### Cost Metrics
- Cost per request
- Cost per provider
- Total cost per review run
- Cumulative costs over time

## Error Logging

Errors are logged with detailed context for debugging:

```json
{
  "timestamp": "2025-10-21T12:35:00Z",
  "level": "error",
  "type": "error",
  "provider": "openai",
  "model": "gpt-4o",
  "duration": "0.5s",
  "errorType": "rate_limit",
  "statusCode": 429,
  "retryable": true,
  "message": "Rate limit exceeded. Retry after 60s"
}
```

Error types:
- `authentication`: Invalid API key or permissions
- `rate_limit`: Too many requests (retryable)
- `invalid_request`: Bad request parameters
- `service_unavailable`: Provider outage (retryable)
- `timeout`: Request exceeded timeout
- `content_filtered`: Response blocked by content filters
- `model_not_found`: Model doesn't exist
- `unknown`: Unexpected error

## Metrics Storage

Currently, metrics are tracked in-memory during execution. They are available in:

1. **Review History Database** (`~/.config/bop/reviews.db`)
   - Total cost per run
   - Timestamp and scope
   - Provider and model used

2. **Output Files**
   - Markdown: Cost shown in header
   - JSON: `cost` field in review object
   - SARIF: Cost in run properties

## Use Cases

### Debugging API Issues

Enable debug logging to see full request/response cycle:

```yaml
observability:
  logging:
    enabled: true
    level: debug
    format: human
```

### Cost Monitoring

Track spending across providers:

```bash
# Check last review cost
cr review branch HEAD~1..HEAD

# View output (Markdown includes cost)
cat out/repo_feature_merged_*.md | grep "Cost:"

# Query database for historical costs
sqlite3 ~/.config/bop/reviews.db "SELECT SUM(total_cost) FROM runs;"
```

### Performance Analysis

Use JSON logs with log aggregation tools:

```yaml
observability:
  logging:
    enabled: true
    level: info
    format: json
```

Pipe to `jq` for analysis:

```bash
# Average response time per provider
cat logs.json | jq -s 'group_by(.provider) | map({provider: .[0].provider, avg_duration: (map(.duration) | add / length)})'

# Total cost per provider
cat logs.json | jq -s 'group_by(.provider) | map({provider: .[0].provider, total_cost: (map(.cost // 0) | add)})'
```

### Production Monitoring

Disable verbose logging in production:

```yaml
observability:
  logging:
    enabled: true
    level: error    # Only log errors
    format: json    # Machine-readable
  metrics:
    enabled: true
```

## Best Practices

1. **Keep API Key Redaction Enabled**: Always use `redactAPIKeys: true` to prevent credential leaks

2. **Use Appropriate Log Levels**:
   - Development: `debug` for troubleshooting
   - Production: `error` for alerting
   - Default: `info` for general monitoring

3. **Choose the Right Format**:
   - `human`: For manual inspection and debugging
   - `json`: For log aggregation and automated analysis

4. **Monitor Costs**: Regularly check cost metrics to avoid unexpected bills

5. **Review Error Patterns**: Analyze error logs to identify provider issues or rate limits

## Troubleshooting

### No logs appearing

Check if logging is enabled:
```bash
echo $CR_OBSERVABILITY_LOGGING_ENABLED  # Should be true or empty
```

### Logs are too verbose

Reduce log level:
```yaml
observability:
  logging:
    level: error  # Only errors
```

### Want to see API keys for debugging

Temporarily disable redaction (be careful!):
```bash
CR_OBSERVABILITY_LOGGING_REDACT_API_KEYS=false bop review ...
```

**Warning**: Only do this in secure environments. Never commit logs with exposed API keys.

### Metrics not tracking

Verify metrics are enabled:
```yaml
observability:
  metrics:
    enabled: true
```

## Future Enhancements

Planned observability features:

- Export metrics to Prometheus
- OpenTelemetry integration
- Distributed tracing for multi-provider reviews
- Real-time dashboards
- Alert thresholds for costs and errors
- Detailed latency percentiles (p50, p95, p99)

## See Also

- [COST_TRACKING.md](./COST_TRACKING.md) - Detailed cost tracking guide
- [CONFIGURATION.md](./CONFIGURATION.md) - Full configuration reference
- [HTTP_CLIENT_TODO.md](./HTTP_CLIENT_TODO.md) - Implementation roadmap
