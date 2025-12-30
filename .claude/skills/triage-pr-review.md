# PR Code Review Triage Skill

Triage, respond to, and address PR code review feedback using the `code-reviewer` MCP server.

## Prerequisites

The `code-reviewer` MCP server must be running and configured in Claude Code. It provides 12 tools for PR triage.

## Understanding the Two Sources

**CRITICAL:** There are TWO sources of findings with different behaviors:

| Source | MCP Tools | Behavior |
|--------|-----------|----------|
| **SARIF Annotations** | `list_annotations`, `get_annotation` | Reset each push; shows CURRENT issues only |
| **PR Comments** | `list_findings`, `get_finding` | Accumulate across commits; contains historical noise |

**Always query both sources.** SARIF annotations are authoritative for current code state; PR comments show accumulated reviewer feedback.

---

## Available MCP Tools

### Read Tools (Information Gathering)

| Tool | Purpose | Key Parameters |
|------|---------|----------------|
| `list_annotations` | SARIF findings for HEAD commit | `owner`, `repo`, `pr_number`, optional `level`/`check_name` filters |
| `get_annotation` | Single annotation details | `owner`, `repo`, `check_run_id`, `index` |
| `list_findings` | PR comment findings | `owner`, `repo`, `pr_number`, optional `severity`/`category` filters |
| `get_finding` | Single finding with thread | `owner`, `repo`, `pr_number`, `finding_id` (fingerprint or comment ID) |
| `get_suggestion` | Extract structured code fix | `owner`, `repo`, `pr_number`, `finding_id` |
| `get_code_context` | Current file content at lines | `owner`, `repo`, `pr_number`, `file`, `start_line`, `end_line` |
| `get_diff_context` | Diff hunk at location | `owner`, `repo`, `pr_number`, `file`, `start_line`, `end_line` |

### Write Tools (Actions)

| Tool | Purpose | Key Parameters |
|------|---------|----------------|
| `get_thread` | Full comment thread history | `owner`, `repo`, `comment_id` |
| `reply_to_finding` | Reply to PR comment with status | `owner`, `repo`, `pr_number`, `finding_id`, `body`, optional `status` |
| `post_comment` | New comment at file/line (for SARIF) | `owner`, `repo`, `pr_number`, `file`, `line`, `body` |
| `mark_resolved` | Mark thread as resolved | `owner`, `repo`, `thread_id`, `resolved` |
| `request_rereview` | Dismiss stale reviews, request fresh | `owner`, `repo`, `pr_number`, `dismiss_stale`, optional `reviewers` |

### Status Tags for `reply_to_finding`

Use the `status` parameter to tag your response:
- `acknowledged` - Noted, will address later
- `disputed` - Won't fix, with explanation
- `fixed` - Addressed in code
- `wont_fix` - Intentionally not addressing

---

## Triage Workflow

### Step 1: Gather All Findings

**Always check both sources:**

```
# SARIF annotations (authoritative for current code)
list_annotations(owner, repo, pr_number)

# PR comment findings (accumulated reviewer feedback)
list_findings(owner, repo, pr_number)
```

If either returns empty, explicitly note "0 findings from [source]" in your summary.

### Step 2: Categorize Findings

For each finding, determine the action:

| Category | Action | Examples |
|----------|--------|----------|
| **Errors/Failures** | Must fix | SARIF errors, blocking check failures |
| **Security Issues** | Must fix | Vulnerabilities, injection risks |
| **Bugs** | Should fix | Logic errors, null handling |
| **Valid Suggestions** | Consider fixing | Performance, clarity improvements |
| **Design Disputes** | Reply with explanation | Intentional patterns, architecture decisions |
| **False Positives** | Reply with context | Incorrect analysis, missing context |

### Step 3: Get Details for Complex Findings

```
# Get full finding with thread context
get_finding(owner, repo, pr_number, finding_id)

# Get current code at the location
get_code_context(owner, repo, pr_number, file, start_line, end_line)

# Get structured suggestion for applying fix
get_suggestion(owner, repo, pr_number, finding_id)
```

### Step 4: Apply Fixes

For findings you're addressing:

1. Use `get_suggestion` to extract the proposed fix
2. Apply the fix using standard file editing
3. Run validation: `go test ./... && go build -o cr ./cmd/cr`
4. Commit locally (don't push yet)

### Step 5: Respond to Findings

**For PR comment findings:**
```
reply_to_finding(owner, repo, pr_number, finding_id, body, status="fixed")
```

**For SARIF annotations** (can't reply directly - create new comment):
```
post_comment(owner, repo, pr_number, file, line, body)
```

**Good reply patterns:**
- **Fixed:** "Addressed in commit [hash]. [Brief description of fix]."
- **Disputed:** "This is intentional. [Pattern] is used because [reason]."
- **Won't fix:** "Acceptable risk because [reason]. Cost of fix outweighs benefit."
- **False positive:** "[Code] actually does [X], not [Y]. The [context] ensures safety."

### Step 6: Mark Threads Resolved

After addressing a finding:
```
mark_resolved(owner, repo, thread_id, resolved=true)
```

### Step 7: Push and Request Re-review

After all fixes are committed and responses posted:

```bash
git push
```

Then request fresh review:
```
request_rereview(owner, repo, pr_number, dismiss_stale=true)
```

---

## Decision Matrix

```
Is it a blocking error (SARIF error, check failure)?
  YES -> Fix immediately
  NO  -> Continue...

Is it a security vulnerability?
  YES -> Fix immediately
  NO  -> Continue...

Is it a real bug or logic error?
  YES -> Fix it
  NO  -> Continue...

Is it about intentional design or architecture?
  YES -> Reply with explanation (cite clean architecture, SOLID, etc.)
  NO  -> Continue...

Is it a false positive or lacks context?
  YES -> Reply explaining why
  NO  -> Continue...

Is the fix worth the code churn?
  YES -> Fix it
  NO  -> Reply noting it's deferred
```

---

## Output Format

After triaging, provide a summary:

```markdown
## PR Triage Summary

### Sources Checked
- [x] SARIF Annotations: X findings
- [x] PR Comments: Y findings

### Findings by Source

#### SARIF Annotations
| # | File:Line | Severity | Finding | Decision |
|---|-----------|----------|---------|----------|
| 1 | path:42 | error | [description] | Fixed |

#### PR Comments
| # | File:Line | Finding ID | Finding | Decision |
|---|-----------|------------|---------|----------|
| 1 | path:100 | CR_FP-xxx | [description] | Disputed |

### Actions Taken
| Decision | Count | Details |
|----------|-------|---------|
| Fixed | X | [list] |
| Disputed | Y | [list] |
| Deferred | Z | [list] |

### Status
- Blocking errors: X fixed, Y remaining
- Ready for re-review: [yes/no]
```

---

## Common Dispute Patterns

### Clean Architecture
> "Following clean architecture, domain types are pure data. Validation belongs in the use case layer, not the domain."

### Premature Optimization
> "This is a micro-optimization for code called rarely. The [real bottleneck] dominates runtime."

### Test Design
> "This tests [specific behavior], not [other thing]. Testing both together conflates concerns."

### Error Handling
> "Fail-fast is intentional. If [condition], it's a configuration error that should surface immediately."

### Intentional Patterns
> "This follows [pattern name] per [reference]. The apparent [issue] is actually [explanation]."

---

## Fallback: Raw GitHub API

If MCP tools are unavailable, fall back to `gh api`:

```bash
# Get HEAD commit
HEAD_SHA=$(gh pr view --json headRefOid -q '.headRefOid')

# Find check run
CHECK_RUN_ID=$(gh api repos/{owner}/{repo}/commits/${HEAD_SHA}/check-runs \
  --jq '.check_runs[] | select(.name == "review") | .id' | head -1)

# Get annotations
gh api repos/{owner}/{repo}/check-runs/${CHECK_RUN_ID}/annotations

# Get PR comments
gh api repos/{owner}/{repo}/pulls/{pr}/comments
```

---

## After Loading This Skill

1. Identify current PR: `owner`, `repo`, `pr_number`
2. Query both sources using MCP tools
3. Present findings by source in separate tables
4. Categorize each: fix, dispute, or defer
5. Apply fixes and respond to findings
6. Push and request re-review
