# GitHub Action Setup Guide

This guide explains how to enable AI-powered code reviews in your GitHub repository using GitHub Actions and Code Scanning.

## ⚠️ Security Warning - Read First

**This workflow sends your code diffs to third-party LLM APIs.**

Before enabling this workflow, please understand:

### Critical Security Considerations

1. **Code Transmission**: All code changes in PRs are sent to your configured LLM provider (OpenAI, Anthropic, Google, etc.)
2. **Data Retention**: Providers may retain your code for 30 days or longer (varies by provider and tier)
3. **Secret Exposure Risk**: While the tool has secret redaction, it cannot catch all secret formats
4. **Proprietary Code**: Your algorithms and business logic become visible to the LLM provider

### Required Actions Before Enabling

- [ ] **Read [SECURITY.md](SECURITY.md)** - Complete security documentation
- [ ] **Review provider policy** - Understand data retention for your chosen LLM provider
- [ ] **Check compliance** - Verify tool usage complies with your org's policies (GDPR, HIPAA, etc.)
- [ ] **Never commit secrets** - Ensure your team follows secret management best practices
- [ ] **Consider Ollama** - For sensitive repos, use local Ollama models instead of cloud APIs

### Recommendations by Repository Type

**Public Open-Source Repositories:**
- ✅ Generally safe to use
- Use cheaper models (gpt-4o-mini, claude-3-5-haiku)

**Private Repositories (Personal):**
- ⚠️ Acceptable with standard API tiers
- Review your provider's data retention policy
- Avoid reviewing files with secrets or sensitive data

**Private Repositories (Company/Enterprise):**
- ⚠️ **Requires approval** from InfoSec/Legal
- **Required**: Enterprise LLM tier with data protection guarantees
- **Alternative**: Use local Ollama models (no data leaves your infrastructure)
- Set up `redaction.denyGlobs` to exclude sensitive files

**Do NOT use without approval on:**
- Repositories with regulated data (HIPAA, PCI-DSS)
- Repositories with customer PII
- Repositories with security-critical code
- Repositories subject to export controls

See [SECURITY.md](SECURITY.md) for complete security considerations.

---

## Overview

The `.github/workflows/code-review.yml` workflow automatically runs the code reviewer on every pull request and posts the results in two ways:

1. **PR Comment**: Full review summary posted as a comment on the PR
2. **Code Scanning**: SARIF upload for inline annotations on specific lines

This enables:

- **Review summaries** as PR comments for easy overview and discussion
- **Inline annotations** on the "Files changed" tab of pull requests
- **Security alerts** in the GitHub Security tab
- **Historical tracking** of review findings across PRs
- **Claude Code integration** - Review PR comments with Claude Code and take action

## Prerequisites

1. **GitHub repository** with:
   - Code Scanning enabled (available on public repos and GitHub Enterprise)
   - Advanced Security enabled (for private repos - requires GitHub Enterprise)

2. **OpenAI API key** (or other LLM provider API key)

## Setup Instructions

### 1. Add API Key to GitHub Secrets

1. Navigate to your repository on GitHub
2. Go to **Settings** → **Secrets and variables** → **Actions**
3. Click **New repository secret**
4. Name: `OPENAI_API_KEY`
5. Value: Your OpenAI API key
6. Click **Add secret**

### 2. Enable Code Scanning

Code Scanning should be automatically enabled when you push the workflow file. To verify:

1. Go to your repository on GitHub
2. Navigate to **Security** → **Code scanning**
3. You should see "AI Code Review" as one of the analysis tools

### 3. Test the Workflow

Create a test pull request:

```bash
git checkout -b test-code-review
echo "// Test change" >> cmd/bop/main.go
git add cmd/bop/main.go
git commit -m "Test: Trigger code review workflow"
git push origin test-code-review
```

Then create a PR from `test-code-review` to `main` on GitHub.

### 4. View Results

After the workflow runs (typically 2-5 minutes):

1. **In PR Comments**:
   - Scroll to the PR conversation tab
   - Look for "🤖 AI Code Review" comment
   - Contains full review summary with all findings
   - Easy to read, discuss, and review with Claude Code

2. **In the PR Files**:
   - Go to the **Files changed** tab
   - Look for inline annotations on specific lines
   - Each annotation represents a finding from the AI reviewer
   - Click annotation for details

3. **In Security Tab**:
   - Navigate to **Security** → **Code scanning**
   - View all findings with severity levels
   - Filter by branch, alert type, or status

4. **In Workflow Artifacts**:
   - Go to **Actions** tab
   - Click on the "AI Code Review" workflow run
   - Download the "code-review-results" artifact
   - Contains SARIF, Markdown, and formatted comment files

## Workflow Details

### When It Runs

The workflow triggers on:
- `pull_request` events (opened, synchronize, reopened)
- Only for PRs targeting the `main` branch

### What It Does

1. **Checks out code** with full git history
2. **Builds the tool** from source (ensures latest version)
3. **Runs code review once**:
   - Tool automatically generates Markdown, JSON, and SARIF outputs
4. **Posts review comment** to PR with full findings (uses Markdown)
5. **Uploads to Code Scanning** for inline annotations (uses SARIF)
6. **Archives all outputs** as artifacts for 30 days

### Permissions Required

The workflow needs:
- `contents: read` - Read repository code
- `security-events: write` - Upload SARIF to Code Scanning
- `pull-requests: write` - Post review comments on PRs

**Important**: To allow the bot to post `APPROVE` or `REQUEST_CHANGES` reviews (not just comments), you must enable this repository setting:

1. Go to **Settings** → **Actions** → **General**
2. Scroll to **Workflow permissions**
3. Check **"Allow GitHub Actions to create and approve pull requests"**
4. Click **Save**

Without this setting, reviews will fail with a 422 error when attempting to approve PRs. This is a GitHub security feature to prevent malicious PRs from self-approving.

### Output Formats

The workflow generates two outputs:

1. **Markdown Comment**: Posted as a PR comment with full review summary
   - Includes all findings with descriptions, severity, and file locations
   - Easy to read and discuss in PR conversation
   - Can be reviewed with Claude Code (see "Using Claude Code" section)

2. **SARIF File**: Uploaded to Code Scanning for inline annotations
   - Creates annotations on specific lines in "Files Changed" tab
   - Integrated with GitHub Security tab
   - Standard format for code analysis tools

## Using Claude Code to Review AI Feedback

One of the most powerful workflows is using Claude Code (from your laptop) to review the automated code review comments and take action.

### Workflow

1. **PR Created**: Push your branch and create a PR on GitHub
2. **AI Reviews**: GitHub Action runs and posts review summary as a PR comment
3. **Review with Claude**: Use Claude Code to analyze the AI feedback
4. **Take Action**: Implement fixes, dismiss false positives, or ask for clarification

### Example Commands

```bash
# Clone the PR to your local machine
gh pr checkout 123

# Ask Claude Code to review the AI feedback
# (Claude Code can read the PR comments via gh CLI)
```

Then in Claude Code:

> "Read the AI code review comments from this PR (gh pr view --comments) and help me prioritize which findings to address. Show me the top 3 most critical issues."

> "The AI reviewer flagged a potential SQL injection in `database.go:45`. Is this a real issue or a false positive? If real, help me fix it."

> "Generate a summary of all security-related findings from the AI review and create a TODO list to address them."

### Advanced: Automated Triage

You can create a follow-up workflow that uses Claude Code to automatically triage AI findings:

```bash
# Script: triage-ai-review.sh
#!/bin/bash

# Get PR number
PR_NUM=$1

# Fetch AI review comments
gh pr view $PR_NUM --comments > pr-comments.txt

# Use Claude Code to analyze (requires Claude Code CLI)
claude analyze pr-comments.txt \
  --prompt "Categorize these AI code review findings into:
    1. Critical (must fix before merge)
    2. Important (should fix soon)
    3. Nice to have (can defer)
    4. False positives (can ignore)
  Output as a markdown checklist."
```

### Benefits

- **Human-in-the-loop**: AI suggests, human decides
- **Context-aware triage**: Claude Code understands your codebase
- **Faster iteration**: Implement fixes with AI assistance
- **Learning opportunity**: Understand why AI flagged certain patterns

## Configuration

### Using a Different LLM Provider

To use Anthropic, Gemini, or Ollama instead of OpenAI:

1. **Add provider API key** to GitHub Secrets:
   - `ANTHROPIC_API_KEY` for Claude
   - `GEMINI_API_KEY` for Google Gemini
   - No key needed for Ollama (local only, not suitable for GitHub Actions)

2. **Create a `.bop.yaml` config file** in your repository:

```yaml
# .bop.yml
providers:
  anthropic:
    apiKey: ${ANTHROPIC_API_KEY}
    model: claude-3-5-sonnet-20241022

# Or for Gemini
providers:
  gemini:
    apiKey: ${GEMINI_API_KEY}
    model: gemini-1.5-pro
```

3. **Update workflow** to pass environment variable:

```yaml
- name: Run AI code review
  env:
    ANTHROPIC_API_KEY: ${{ secrets.ANTHROPIC_API_KEY }}
  run: |
    ./bop branch ${{ github.event.pull_request.base.ref }} \
      --format sarif \
      --output review.sarif
```

### Customizing Review Behavior

Create a `.bop.yaml` config file in your repository root:

```yaml
# Example configuration
max_findings: 50
severity_threshold: medium

prompts:
  system: |
    Focus on security vulnerabilities and performance issues.
    Ignore style/formatting issues.

providers:
  openai:
    apiKey: ${OPENAI_API_KEY}
    model: gpt-4o-mini  # Use cheaper model for PR reviews
    temperature: 0.0

planning:
  enabled: false  # Disable interactive planning in CI/CD
```

## Skipping Code Review

You can skip code review by including a skip trigger in:

- **Commit message** - The head (latest) commit in the PR
- **PR title** - The pull request title
- **PR description** - The pull request body/description

### Supported Skip Trigger Patterns

The following patterns are recognized (case-insensitive):

```
[skip code-review]
[skip-code-review]
```

### Examples

**Commit message:**
```bash
git commit -m "docs: update README [skip code-review]"
```

**PR title:**
```
WIP: Draft feature [skip code-review]
```

**PR description:**
```markdown
## Description

This is a work-in-progress PR for the new feature.

[skip code-review]

## Changes

- Initial scaffolding
- Not ready for review yet
```

### Common Use Cases

| Use Case | Where to Add Skip Trigger |
|----------|---------------------------|
| Draft PRs | PR title or description |
| Documentation-only changes | Commit message |
| WIP commits | Commit message |
| Automated dependency updates | PR description (via bot config) |
| Trivial formatting changes | Commit message |

### Workflow Behavior

When a skip trigger is detected:

1. The workflow exits early with success status
2. No review is posted to the PR
3. No SARIF is uploaded to Code Scanning
4. The workflow run shows all skipped steps clearly

This ensures the PR is not blocked by a missing review while still showing that the workflow ran successfully.

## Troubleshooting

### Workflow Fails with "API Key Not Found"

**Solution**: Verify the secret name matches exactly:
- Secret name in GitHub: `OPENAI_API_KEY`
- Reference in workflow: `${{ secrets.OPENAI_API_KEY }}`

### SARIF Upload Fails

**Error**: "Code Scanning is not enabled"

**Solution**:
- For public repos: Should work automatically
- For private repos: Requires GitHub Advanced Security
- Contact your GitHub organization admin to enable

### Review Fails with "GitHub Actions is not permitted to approve"

**Error**: `HTTP 422: GitHub Actions is not permitted to approve pull request`

**Cause**: GitHub Actions cannot approve PRs by default (security feature to prevent self-approving malicious PRs).

**Solution**:
1. Go to **Settings** → **Actions** → **General**
2. Scroll to **Workflow permissions**
3. Check **"Allow GitHub Actions to create and approve pull requests"**
4. Click **Save**

This is required for `APPROVE` and `REQUEST_CHANGES` review events. Without it, the bot can only post `COMMENT` reviews.

### No Comment Posted to PR

**Error**: "Resource not accessible by integration" or "403 Forbidden"

**Solution**:
- Verify workflow has `pull-requests: write` permission
- Check that Actions have permission to write to PRs:
  - Go to **Settings** → **Actions** → **General**
  - Under "Workflow permissions", ensure "Read and write permissions" is selected
  - Click "Save"

**Check**:
1. Workflow "Post review summary" step completed successfully
2. Look in PR conversation tab (not Files changed)
3. Comment may take 30-60 seconds to appear after workflow completes

### No Annotations Appear in PR

**Check**:
1. Workflow completed successfully (green checkmark)
2. SARIF file was uploaded (check step logs)
3. Refresh the "Files changed" tab
4. Check **Security** → **Code scanning** for findings

### Review Takes Too Long

**Solutions**:
- Use a faster model (e.g., `gpt-4o-mini` instead of `gpt-4o`)
- Reduce `max_findings` in config
- Add file size limits in config
- Consider reviewing only changed files (current behavior)

## Cost Considerations

Each PR review incurs LLM API costs:
- **gpt-4o-mini**: ~$0.01-0.10 per review (typical PR)
- **gpt-4o**: ~$0.10-1.00 per review
- **claude-3-5-haiku**: ~$0.01-0.05 per review
- **claude-3-5-sonnet**: ~$0.05-0.50 per review

**Note**: The tool generates all output formats (Markdown, JSON, SARIF) from a single review run, so you only pay for one API call per PR, not multiple.

**Recommendations**:
- Use cheaper models for routine PRs (`gpt-4o-mini`, `claude-3-5-haiku`)
- Reserve expensive models for critical reviews
- Monitor costs via LLM provider dashboard
- Set `max_findings` limits to control token usage

## Next Steps

Once self-dogfooding is working well, we plan to add:

1. **Inline PR comments** (v0.3.0) - Direct comments on code lines instead of just annotations
2. **Finding deduplication** - Remember past findings to avoid repeat comments
3. **Cost tracking** - Show per-PR review costs in summary
4. **Custom review rules** - Repository-specific review criteria
5. **Multi-provider ensemble** - Combine insights from multiple models

## Support

For issues or questions:
- Check workflow logs in GitHub Actions tab
- Review SARIF artifact for detailed output
- Consult [CONFIGURATION.md](./CONFIGURATION.md) for config options
- Open an issue in the bop repository
