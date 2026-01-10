# PR Code Review Triage Skill

Triage, respond to, and address PR code review feedback using the `bop` MCP server.

## Prerequisites

The `bop` MCP server must be running and configured in Claude Code. It provides 12 tools for PR triage.

## Understanding the Two Sources

**CRITICAL:** There are TWO sources of findings with different behaviors:

| Source | MCP Tools | Behavior |
|--------|-----------|----------|
| **SARIF Annotations** | `list_annotations`, `get_annotation` | Reset each push; shows CURRENT issues only |
| **PR Comments** | `list_findings`, `get_finding` | Accumulate across commits; contains historical noise |

**Always query both sources.** SARIF annotations are authoritative for current code state; PR comments show accumulated reviewer feedback.

### Bot Comment Handling

`list_findings` returns both:
- Comments with `CR_FP` fingerprint markers (from bop)
- **All comments from bot users** (e.g., `github-actions[bot]`, `github-advanced-security[bot]`)

This ensures all automated review feedback is captured, regardless of whether the bot uses fingerprints.

The `finding_id` parameter in `reply_to_finding` and `get_finding` accepts both fingerprints (`CR_FP:xxx`) and raw comment IDs, so you can respond to any finding.

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

| Status | When to Use | Typical Action |
|--------|-------------|----------------|
| `fixed` | Issue addressed in code | Describe the fix applied |
| `acknowledged` | Valid observation, will address later | Create tracking issue, reference in reply |
| `disputed` | Finding is incorrect or based on misunderstanding | Explain why the analysis is wrong |
| `wont_fix` | Intentional design choice, not changing | Explain the deliberate tradeoff |

**Key distinction: `acknowledged` vs `disputed`**
- **Acknowledged:** The finding is *correct* but out of scope for this PR. Create an issue to track follow-up.
- **Disputed:** The finding is *incorrect* - false positive, wrong analysis, or misunderstands the code.

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

For each finding, determine the category:

| Category | Action | Examples |
|----------|--------|----------|
| **Needs Fix** | Fix now | Security vulnerabilities, bugs, blocking errors |
| **Already Addressed** | Reply noting prior fix | Fixed in earlier round, code already handles this |
| **Already Tracked** | Reply with issue link | Scalability concern tracked in Issue #XX |
| **Acknowledge + Track** | Create issue, reply with link | Valid but out of scope for this PR |
| **Acknowledge** | Reply accepting tradeoff | Minor observation, acceptable as-is, no follow-up needed |
| **Disputed** | Reply with explanation | False positive, incorrect analysis, misunderstands code |

**When to Create Tracking Issues:**

Create a GitHub issue when a finding is:
- ✅ Valid and correct (not a false positive)
- ✅ Worth addressing eventually (not trivial)
- ❌ Out of scope for the current PR (scope creep, different concern)

Examples that warrant issues:
- "Config inconsistency" when PR adds new feature, not refactoring config
- "Missing tests for edge case X" when PR focuses on different functionality
- "Performance optimization Y" when PR addresses correctness, not performance

> **Don't dispute valid observations.** If the reviewer is correct but the work doesn't belong in this PR, acknowledge it with a tracking issue. Reserve "disputed" for findings that are genuinely incorrect.

### Step 3: Get Details for Complex Findings

```
# Get full finding with thread context
get_finding(owner, repo, pr_number, finding_id)

# Get current code at the location
get_code_context(owner, repo, pr_number, file, start_line, end_line)

# Get structured suggestion for applying fix
get_suggestion(owner, repo, pr_number, finding_id)
```

### Step 4: Respond to ALL Findings FIRST

**IMPORTANT:** Reply to every finding BEFORE making code changes. This creates a clear audit trail showing you evaluated each finding before acting.

**For PR comment findings:**
```
reply_to_finding(owner, repo, pr_number, finding_id, body, status="fixed")
```

**For SARIF annotations** (can't reply directly - create new comment):
```
post_comment(owner, repo, pr_number, file, line, body)
```

**Reply patterns by category:**
- **Fixed:** "Will fix. [description of planned fix]." or "Fixed in [commit]."
- **Already Addressed:** "Already addressed in [previous commit/fix]. [Brief explanation]."
- **Already Tracked:** "Already tracked in Issue #XX. [Brief context]."
- **Acknowledge + Track:** "Valid observation. Tracked in #XX for follow-up." (Create issue first!)
- **Acknowledged:** "This is intentional/acceptable. [Explanation of tradeoff]."
- **Disputed:** "This is incorrect because [explanation of why the analysis is wrong]."

> **Tip:** For "Acknowledge + Track", create the issue first, then reply with the issue number. This ensures the finding is properly tracked before you move on.

### Step 5: Apply Fixes

For findings you're addressing:

1. Use `get_suggestion` to extract the proposed fix (if available)
2. Apply the fix using standard file editing (bottom-up for same file)
3. Run **all quality gates**: `mage fmt && mage lint && mage test && mage build`
4. Commit locally with descriptive message

### Step 6: Push and Re-Request Review

After all fixes are committed:

```bash
git push
```

Then dismiss stale reviews (especially if blocking findings were addressed):
```
request_rereview(owner, repo, pr_number, dismiss_stale=true,
                 message="Blocking findings addressed: [summary]")
```

### Step 7: Mark Threads Resolved (Optional)

After a finding is fully addressed and verified:
```
mark_resolved(owner, repo, thread_id, resolved=true)
```

Note: This is optional - many teams prefer to leave threads open for visibility.

### Step 8: Iterate Until Clean

**The triage process is iterative.** Each push triggers a new review round that may find new issues. Repeat steps 1-7 until:
- No new findings appear, OR
- All remaining findings are acknowledged/tracked

Typical PRs may require 3-8 rounds before all findings are addressed or triaged.

---

## Decision Matrix

```
Was this already fixed in a previous round?
  YES -> Reply "Already addressed" with context
  NO  -> Continue...

Is there already a tracking issue for this concern?
  YES -> Reply "Already tracked in Issue #XX"
  NO  -> Continue...

Is it a blocking error, security issue, or real bug?
  YES -> Fix immediately
  NO  -> Continue...

Is it a FALSE POSITIVE or INCORRECT analysis?
  (Reviewer misunderstands the code, wrong assumptions, factual error)
  YES -> Reply "Disputed" with explanation of why analysis is wrong
  NO  -> Continue...

Is it VALID but OUT OF SCOPE for this PR?
  (Scope creep, different concern, would touch unrelated code)
  YES -> Create tracking issue, reply "Acknowledged" with issue link
  NO  -> Continue...

Is it a minor observation, acceptable as-is?
  (True but trivial, documented tradeoff, intentional design)
  YES -> Reply "Acknowledged" or "Won't fix" with brief explanation
  NO  -> Fix it
```

> **Key principle:** If the finding is *correct*, don't dispute it. Either fix it, acknowledge it with a tracking issue, or explain why it's an acceptable tradeoff. Reserve "disputed" for findings that are genuinely *wrong*.

---

## Output Format

After triaging each round, provide a summary:

```markdown
## Round N Triage - X new findings

| Category | Count | IDs |
|----------|-------|-----|
| Needs fix | N | ID1 (description), ID2 (description) |
| Acknowledge + Track | N | ID3 (→#XX new), ID4 (→#YY new) |
| Already tracked | N | ID5 (→#ZZ) |
| Already addressed | N | ID6, ID7 |
| Acknowledged | N | ID8, ID9 |
| Disputed | N | ID10 |

**Fixes needed:**
1. ID1 - [brief description of issue and fix]
2. ID2 - [brief description of issue and fix]

**Issues created:**
- #XX - [title] (for ID3)
- #YY - [title] (for ID4)

**Investigation required:** (if any findings need deeper analysis)
- ID11 - Need to verify [specific concern]
```

After completing a round:
```markdown
**Round N Summary:**
| Fix | Status |
|-----|--------|
| [description] | ✅ Fixed |
| [description] | ✅ Fixed |

All X findings replied to (Y fixed, Z acknowledged). Changes pushed.
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

If MCP tools fail (e.g., pagination bug with `list_findings`), fall back to `gh api`:

```bash
# Get HEAD commit
HEAD_SHA=$(gh pr view --json headRefOid -q '.headRefOid')

# Find check run
CHECK_RUN_ID=$(gh api repos/{owner}/{repo}/commits/${HEAD_SHA}/check-runs \
  --jq '.check_runs[] | select(.name == "review") | .id' | head -1)

# Get annotations
gh api repos/{owner}/{repo}/check-runs/${CHECK_RUN_ID}/annotations

# Get PR comments (with pagination)
gh api repos/{owner}/{repo}/pulls/{pr}/comments --paginate

# Get bot findings with key fields
gh api repos/{owner}/{repo}/pulls/{pr}/comments --paginate \
  --jq '.[] | select(.user.login | test("bop-bot|github-actions")) | {id, path, line, body: .body[0:100]}'

# Filter for specific round (by ID range)
gh api repos/{owner}/{repo}/pulls/{pr}/comments --paginate \
  --jq '.[] | select(.id >= 2670394000) | {id, path, severity: (.body | capture("Severity:.* (?<s>[a-z]+)") | .s)}'
```

---

## Tips & Best Practices

### Apply Edits Bottom-Up

When fixing multiple findings in the same file, **always apply edits from bottom to top** (highest line numbers first). This prevents line number drift as edits change file length.

```
# BAD: Top-down (line numbers shift)
Fix finding at line 10  -> File grows by 2 lines
Fix finding at line 50  -> Now it's actually line 52!

# GOOD: Bottom-up (line numbers stable)
Fix finding at line 50  -> Line 10 unaffected
Fix finding at line 10  -> Correct location
```

### Triage Workflow Order

**The correct order is: respond → fix → commit → push → re-request**

| Step | Action | Why This Order |
|------|--------|----------------|
| 1 | Gather all findings | Understand full scope before acting |
| 2 | Analyze and categorize | Determine fix vs dispute vs acknowledge |
| 3 | **Respond to findings** | Creates paper trail BEFORE code changes |
| 4 | Apply code fixes | Fix valid issues locally |
| 5 | Run quality gates | `mage fmt && mage lint && mage test` |
| 6 | Commit locally | Group related fixes |
| 7 | Push | Triggers new review cycle |
| 8 | Re-request review | Dismiss stale reviews if blocking findings addressed |

> ⚠️ **CRITICAL: Respond BEFORE Committing/Pushing**
>
> Always post responses to findings (step 3) BEFORE committing fixes (step 6). This ordering matters for several reasons:
>
> 1. **Paper trail:** Responses are anchored to the commit they were raised on, showing you evaluated each finding before acting
> 2. **Race prevention:** Pushing triggers CI which may start a new review cycle before your responses post
> 3. **Clean audit:** Reviewers see your reasoning alongside the original finding, not after the fix
>
> **Anti-pattern:** fix → commit → push → respond (responses arrive after new findings, creating confusion)

### Re-Requesting Review

After addressing **blocking findings** (fixed or disputed with rationale), re-request review:

```
request_rereview(owner, repo, pr_number, dismiss_stale=true,
                 message="Blocking findings addressed: [summary]")
```

This dismisses stale bot reviews and allows the PR to proceed. Only do this when:
- All blocking findings have been fixed, OR
- Blocking findings have been disputed with clear rationale

### Batch Similar Operations

For efficiency, group operations by type within each step:

- **Gather phase:** Query `list_annotations` and `list_findings` in parallel
- **Context phase:** Batch `get_code_context` calls for all findings needing investigation
- **Response phase:** Post all `reply_to_finding` calls before any commits
- **Fix phase:** Apply all edits bottom-up (highest line numbers first)
- **Quality phase:** Run all gates once after all fixes: `mage fmt && mage lint && mage test`

### Use Native Tools Together with MCP

The MCP server provides **finding discovery and GitHub interaction**. Combine with native tools:

| Task | Tool |
|------|------|
| Get findings | MCP: `list_annotations`, `list_findings` |
| Read current code | MCP: `get_code_context` or native `Read` tool |
| Edit files | Native `Edit` tool (not MCP) |
| Run tests | Native `Bash` tool: `go test ./...` |
| Commit changes | Native `Bash` tool: `git commit` |
| Respond to findings | MCP: `reply_to_finding`, `post_comment` |

### Handle Edge Cases

| Situation | Action |
|-----------|--------|
| Finding on deleted file | Skip fix, reply explaining file was removed |
| Line numbers out of bounds | Request re-review; file has changed |
| No suggestion available | Use `get_code_context` and craft fix manually |
| Thread already resolved | No action needed (idempotent) |
| Rate limited | Wait and retry; surface to user if persistent |

### Prioritize by Severity

Address findings in this order:
1. **Blocking:** SARIF errors, check failures (must fix to merge)
2. **Critical/High:** Security issues, bugs
3. **Medium:** Code quality, performance
4. **Low:** Style, minor improvements

---

## After Loading This Skill

1. Identify current PR: `owner`, `repo`, `pr_number`
2. Query both sources using MCP tools
3. Present findings by source in separate tables
4. Categorize each: fix, dispute, or defer
5. Apply fixes and respond to findings
6. Push and request re-review
