# Cost Tracking Guide

This guide explains how code-reviewer calculates, tracks, and reports API costs across different LLM providers.

## Overview

Code-reviewer automatically tracks API costs for all provider interactions:

- ✅ Real-time cost calculation based on token usage
- ✅ Per-provider and per-model pricing
- ✅ Cost aggregation across multiple providers
- ✅ Historical cost tracking in database
- ✅ Cost display in all output formats

## How Cost Tracking Works

### 1. Token Counting

Each LLM provider returns token counts in their API responses:
- **Input tokens**: Size of the prompt sent
- **Output tokens**: Size of the completion received

### 2. Price Calculation

Costs are calculated using official pricing from each provider:

```
Cost = (InputTokens / 1,000,000 × InputPrice) + (OutputTokens / 1,000,000 × OutputPrice)
```

### 3. Cost Aggregation

For multi-provider reviews, costs are summed:

```
TotalCost = OpenAICost + AnthropicCost + GeminiCost + OllamaCost
```

## Provider Pricing

Pricing data accurate as of **October 21, 2025**.

### OpenAI

| Model | Input (per 1M tokens) | Output (per 1M tokens) |
|-------|----------------------|------------------------|
| gpt-4o | $2.50 | $10.00 |
| gpt-4o-mini | $0.15 | $0.60 |
| gpt-4o-2024-11-20 | $2.50 | $10.00 |
| gpt-4o-2024-08-06 | $2.50 | $10.00 |
| gpt-4o-2024-05-13 | $5.00 | $15.00 |
| gpt-4-turbo | $10.00 | $30.00 |
| gpt-4 | $30.00 | $60.00 |
| gpt-3.5-turbo | $0.50 | $1.50 |
| o1 | $15.00 | $60.00 |
| o1-mini | $3.00 | $12.00 |
| o4 | $15.00 | $60.00 |
| o4-mini | $3.00 | $12.00 |

Source: [OpenAI Pricing](https://openai.com/api/pricing/)

### Anthropic

| Model | Input (per 1M tokens) | Output (per 1M tokens) |
|-------|----------------------|------------------------|
| claude-3-5-sonnet-20241022 | $3.00 | $15.00 |
| claude-3-5-sonnet-20240620 | $3.00 | $15.00 |
| claude-3-5-haiku-20241022 | $1.00 | $5.00 |
| claude-3-opus-20240229 | $15.00 | $75.00 |
| claude-3-sonnet-20240229 | $3.00 | $15.00 |
| claude-3-haiku-20240307 | $0.25 | $1.25 |

Source: [Anthropic Pricing](https://www.anthropic.com/pricing)

### Google Gemini

| Model | Input (per 1M tokens) | Output (per 1M tokens) |
|-------|----------------------|------------------------|
| gemini-1.5-pro | $1.25 | $5.00 |
| gemini-1.5-flash | $0.075 | $0.30 |
| gemini-1.5-flash-8b | $0.0375 | $0.15 |
| gemini-pro | $0.50 | $1.50 |
| gemini-1.0-pro | $0.50 | $1.50 |

Source: [Google AI Pricing](https://ai.google.dev/pricing)

### Ollama (Local)

| Model | Cost |
|-------|------|
| All models | $0.00 (free) |

Ollama runs models locally, so there are no API costs.

## Cost Display

### Markdown Output

Cost appears in the header of Markdown reports:

```markdown
# Code Review Report

- Provider: openai (gpt-4o)
- Base: main
- Target: feature
- Cost: $0.0234

## Summary
...
```

### JSON Output

Cost is included in the review object:

```json
{
  "providerName": "openai",
  "modelName": "gpt-4o",
  "summary": "Review summary...",
  "findings": [...],
  "cost": 0.0234
}
```

### SARIF Output

Cost is in the run properties:

```json
{
  "version": "2.1.0",
  "runs": [{
    "tool": {...},
    "results": [...],
    "properties": {
      "cost": 0.0234,
      "summary": "Review summary...",
      "model": "gpt-4o"
    }
  }]
}
```

### Database Storage

Total costs are stored in the review history database:

```sql
-- Check total spending
SELECT SUM(total_cost) as total_spent
FROM runs;

-- Costs per day
SELECT DATE(timestamp) as date, SUM(total_cost) as daily_cost
FROM runs
GROUP BY DATE(timestamp)
ORDER BY date DESC;

-- Costs by scope (branch comparisons)
SELECT scope, COUNT(*) as reviews, SUM(total_cost) as total_cost
FROM runs
GROUP BY scope
ORDER BY total_cost DESC;

-- Most expensive reviews
SELECT run_id, scope, total_cost, timestamp
FROM runs
ORDER BY total_cost DESC
LIMIT 10;
```

## Cost Examples

### Single Provider Review

Reviewing a medium-sized PR with gpt-4o-mini:

```
Input tokens: 8,500 (PR diff + context)
Output tokens: 1,200 (review findings)

Cost = (8,500/1,000,000 × $0.15) + (1,200/1,000,000 × $0.60)
     = $0.001275 + $0.00072
     = $0.001995 ≈ $0.002
```

### Multi-Provider Review

Running 3 providers on the same PR:

```
gpt-4o:         $0.0234
claude-3-5-sonnet: $0.0198
gemini-1.5-pro:    $0.0087

Total Cost:     $0.0519
```

### Large Codebase Review

Reviewing a large PR with multiple files using gpt-4o:

```
Input tokens: 45,000 (large diff)
Output tokens: 5,000 (detailed findings)

Cost = (45,000/1,000,000 × $2.50) + (5,000/1,000,000 × $10.00)
     = $0.1125 + $0.05
     = $0.1625
```

## Cost Optimization Tips

### 1. Use Cheaper Models for Simple Reviews

For small PRs or style checks:
```yaml
providers:
  openai:
    model: gpt-4o-mini  # ~94% cheaper than gpt-4o
```

### 2. Use Local Models for Development

Free option for testing:
```yaml
providers:
  ollama:
    enabled: true
    model: codellama
```

### 3. Enable Single Provider Mode

Review with one provider instead of multiple:
```yaml
providers:
  openai:
    enabled: true
  anthropic:
    enabled: false  # Disable to save costs
  gemini:
    enabled: false
```

### 4. Filter Files Before Review

Reduce input token count by excluding non-critical files:
```yaml
redaction:
  enabled: true
  denyGlobs:
    - "**/*.generated.go"
    - "**/*.pb.go"
    - "**/vendor/**"
```

### 5. Use Incremental Reviews

Review only changed files instead of full diffs:
```bash
# Review only the last commit
cr review branch HEAD~1..HEAD

# Instead of reviewing entire branch
cr review branch main..feature  # May be large/expensive
```

### 6. Set Budget Limits (Future Feature)

Configure hard caps to prevent overspending:
```yaml
budget:
  hardCapUSD: 10.00  # Stop reviews if cost exceeds $10
  degradationPolicy:
    - "openai:gpt-4o-mini"   # Fallback to cheaper model
    - "ollama:codellama"     # Fallback to local model
```

## Monitoring Costs

### Real-time Monitoring

Check costs immediately after review:

```bash
# Run review and check output
cr review branch HEAD~1..HEAD

# Cost shown in markdown output
cat out/repo_feature_merged_*.md | grep "Cost:"
```

### Historical Analysis

Query the database for trends:

```bash
# Open database
sqlite3 ~/.config/bop/reviews.db

# Weekly costs
SELECT
  strftime('%Y-W%W', timestamp) as week,
  COUNT(*) as reviews,
  ROUND(SUM(total_cost), 4) as total_cost,
  ROUND(AVG(total_cost), 4) as avg_cost
FROM runs
GROUP BY week
ORDER BY week DESC;

# Cost by repository
SELECT
  repository,
  COUNT(*) as reviews,
  ROUND(SUM(total_cost), 4) as total_cost
FROM runs
GROUP BY repository
ORDER BY total_cost DESC;
```

### Cost Alerts

Set up alerts using cron and scripts:

```bash
#!/bin/bash
# check-costs.sh - Alert if daily costs exceed threshold

THRESHOLD=5.00
DAILY_COST=$(sqlite3 ~/.config/bop/reviews.db \
  "SELECT SUM(total_cost) FROM runs WHERE DATE(timestamp) = DATE('now')")

if (( $(echo "$DAILY_COST > $THRESHOLD" | bc -l) )); then
  echo "WARNING: Daily cost $DAILY_COST exceeds threshold $THRESHOLD"
  # Send alert email, Slack message, etc.
fi
```

## Cost Accuracy

### Precision

Costs are calculated to 4 decimal places ($0.0001 precision).

### Variance

Expect costs to match provider bills within ≤5% due to:
- Rounding in token counts
- Cache hits (some providers offer discounts)
- Pricing changes between implementation and billing

### Updates

Pricing data is current as of implementation (October 21, 2025). Provider pricing may change. Check official provider websites for current rates:

- [OpenAI Pricing](https://openai.com/api/pricing/)
- [Anthropic Pricing](https://www.anthropic.com/pricing)
- [Google AI Pricing](https://ai.google.dev/pricing)

## Troubleshooting

### Cost shows $0.00

Possible causes:
1. **Ollama provider**: Local models are always free
2. **Metrics disabled**: Enable in config
3. **Unknown model**: Model not in pricing table

Check which model was used:
```bash
cat out/review-*.json | jq '.modelName'
```

### Cost seems too high

Verify token counts:
```bash
# Check actual tokens used
cat out/review-*.json | jq '{model, tokensIn, tokensOut, cost}'
```

Calculate manually:
```
Cost = (tokensIn/1M × inputPrice) + (tokensOut/1M × outputPrice)
```

### Cost not in output

Ensure observability is enabled:
```yaml
observability:
  metrics:
    enabled: true
```

## Future Enhancements

Planned cost tracking features:

- **Budget enforcement**: Hard caps with automatic degradation
- **Cost forecasting**: Predict costs before running review
- **Provider cost comparison**: Recommend cheapest provider for each review
- **Batch discounts**: Track and apply volume pricing
- **Cost dashboards**: Visual charts and trends
- **Export to accounting**: CSV exports for expense tracking

## See Also

- [OBSERVABILITY.md](./OBSERVABILITY.md) - Logging and metrics guide
- [CONFIGURATION.md](./CONFIGURATION.md) - Configuration reference
- [OBSERVABILITY_COST_DESIGN.md](./OBSERVABILITY_COST_DESIGN.md) - Technical design document
