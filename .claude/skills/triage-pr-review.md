# PR Code Review Triage Skill

Triage, respond to, and address PR code review feedback including SARIF code scanning alerts.

## Instructions

When this skill is invoked (e.g., "triage the latest pr code review findings"), perform a comprehensive assessment of all feedback on the current PR and take appropriate action.

## CRITICAL: Check Run Annotations vs PR Comments

**IMPORTANT:** There are TWO sources of findings, and they behave very differently:

### 1. Check Run Annotations (AUTHORITATIVE)
- **What:** SARIF findings from the LATEST review cycle only
- **Where:** `/check-runs/{id}/annotations`
- **Use for:** Determining what issues exist NOW on the current code
- **Behavior:** Each push creates NEW annotations; old ones don't accumulate

### 2. PR Comments (ACCUMULATED)
- **What:** Inline comments from ALL review cycles across the PR's lifetime
- **Where:** `/pulls/{pr}/comments`
- **Use for:** Responding to human reviewers, checking reply status
- **Behavior:** Comments ACCUMULATE - you'll see stale findings from old commits!

**ALWAYS query check run annotations first** to see the actual current findings. PR comments contain historical noise from previous review cycles.

```bash
# Get the HEAD commit and check run
HEAD_SHA=$(gh pr view --json headRefOid -q '.headRefOid')
CHECK_RUN_ID=$(gh api repos/{owner}/{repo}/commits/${HEAD_SHA}/check-runs \
  --jq '.check_runs[] | select(.name == "openai" or .name == "review") | .id' | head -1)

# Get CURRENT findings from check run annotations (authoritative source)
gh api repos/{owner}/{repo}/check-runs/${CHECK_RUN_ID}/annotations \
  --jq '.[] | {level: .annotation_level, path: .path, line: .start_line, message: .message}'
```

**Why this matters:** If you query PR comments, you'll see 20+ findings from old commits when the actual latest review might only have 3. This leads to triaging already-fixed issues.

---

## Understanding Comment Accumulation (Issue #125 Learnings)

### PR Comments NEVER Disappear

**CRITICAL:** PR comments persist forever, even when:
- The review is dismissed
- The code is fixed
- A new review is posted
- The finding no longer exists in current code

Each review cycle creates NEW comments. Dismissing old reviews only changes their state; inline comments remain visible.

### Why You See "Stale" Findings

The LLM generates slightly different wording each run:
- Run 1: "The MCP **binary constructs** the triage service with all dependencies **set to nil**..."
- Run 2: "The MCP **server is started with** a triage service **constructed with nil** dependencies..."

Different wording → different fingerprints → not deduplicated → duplicate comments posted.

**Semantic deduplication** (Issue #125) catches these, but only when enabled.

### To See Only NEW Comments (After Your Last Push)

```bash
# Get timestamp of your last push
LAST_PUSH=$(git log -1 --format='%aI' HEAD)

# Filter comments created after that time
gh api "repos/{owner}/{repo}/pulls/{pr}/comments?per_page=100" \
  --jq "[.[] | select(.created_at > \"$LAST_PUSH\") | {id, path, created_at, body: .body[0:80]}]"
```

### To Identify Which Review Cycle a Comment Belongs To

```bash
# Group comments by review ID with timestamps
gh api "repos/{owner}/{repo}/pulls/{pr}/comments?per_page=100" \
  --jq 'group_by(.pull_request_review_id) | map({
    review_id: .[0].pull_request_review_id,
    created: .[0].created_at,
    count: length
  })'
```

### Check Workflow Logs for Deduplication Stats

```bash
# Find the latest review workflow run
RUN_ID=$(gh run list --workflow="AI Code Review" --limit 1 --json databaseId -q '.[0].databaseId')

# Check deduplication stats in logs
gh run view $RUN_ID --log 2>&1 | grep -E "duplicate|dedup|skipped|posted"
```

Look for: `commentsPosted=X duplicatesSkipped=Y semanticDuplicatesSkipped=Z`

---

## Pre-Flight Checklist

Before presenting triage results, verify you have checked ALL sources:

- [ ] **Check Run Annotations** - `gh api repos/{owner}/{repo}/check-runs/{id}/annotations`
- [ ] **PR Comments** - `gh api repos/{owner}/{repo}/pulls/{pr}/comments`

**Both must be queried.** If one source has no findings, explicitly state "0 findings from [source]" in your output.

---

## Workflow

### Step 1: Gather Feedback from LATEST Review

**Start with check run annotations (the authoritative source):**

```bash
# Get current PR info
PR_NUMBER=$(gh pr view --json number -q '.number' 2>/dev/null)
HEAD_SHA=$(gh pr view --json headRefOid -q '.headRefOid')

# Check PR status first
gh pr view ${PR_NUMBER} --json state,reviewDecision
gh pr checks ${PR_NUMBER}

# Find the code review check run on HEAD commit
CHECK_RUN_ID=$(gh api repos/{owner}/{repo}/commits/${HEAD_SHA}/check-runs \
  --jq '.check_runs[] | select(.name == "openai" or .name == "review" or .name == "anthropic") | .id' | head -1)

# Get CURRENT findings from check run annotations (THIS IS THE AUTHORITATIVE SOURCE)
gh api repos/{owner}/{repo}/check-runs/${CHECK_RUN_ID}/annotations \
  --jq '.[] | {level: .annotation_level, path: .path, line: .start_line, message: .message}'
```

**Only if needed, check PR comments for human reviewer feedback:**

```bash
# PR comments accumulate across all commits - use sparingly
# Only needed for responding to human reviewers, not for SARIF triage
gh api repos/{owner}/{repo}/pulls/${PR_NUMBER}/comments \
  --jq '.[] | select(.user.type == "User") | {id: .id, path: .path, body: .body[0:100]}'
```

### Step 2: Categorize Findings

Group findings into categories:

| Category | Action | Examples |
|----------|--------|----------|
| **Errors/Failures** | Must fix | SARIF errors, blocking check failures |
| **Security Issues** | Must fix | Vulnerabilities, injection risks, secret exposure |
| **Bugs** | Should fix | Logic errors, edge cases, null handling |
| **Valid Suggestions** | Consider fixing | Performance, clarity improvements |
| **Design Disputes** | Reply inline | Intentional patterns, architecture decisions |
| **False Positives** | Reply inline | Incorrect analysis, missing context |
| **Low Priority** | Defer/note | Micro-optimizations, style preferences |

### Step 3: Triage Decision Matrix

For each finding, apply this decision matrix:

```
Is it a blocking error (SARIF error, check failure)?
  YES → Fix immediately (highest priority)
  NO  → Continue...

Is it a security vulnerability?
  YES → Fix immediately
  NO  → Continue...

Is it a real bug or logic error?
  YES → Fix it
  NO  → Continue...

Is it about intentional design or architecture?
  YES → Reply with explanation (cite principles: clean architecture, SOLID, etc.)
  NO  → Continue...

Is it a false positive or lacks context?
  YES → Reply explaining why (reference existing code, design docs)
  NO  → Continue...

Is the fix worth the code churn?
  YES → Fix it
  NO  → Reply noting it's deferred or accepted risk
```

### Step 4: Address Valid Findings

For findings that need fixing:

1. **Make the fix** - Edit the relevant code
2. **Add tests** - If the fix addresses a bug or edge case
3. **Run validation** - `go test ./... && go build -o cr ./cmd/cr`
4. **Commit with context** - Reference the finding in commit message (but DON'T push yet!)

```bash
git add -A && git commit -m "$(cat <<'EOF'
fix: <description of fix>

Addresses code review finding: <brief description>
EOF
)"
# DO NOT push yet - reply to findings first!
```

**IMPORTANT ORDER OF OPERATIONS:**
1. Make fixes and commit locally
2. Reply to ALL findings on GitHub (Step 5)
3. THEN push changes

This order prevents race conditions where a new review cycle starts before you've responded to the current findings, causing confusion about which findings were addressed.

### Step 5: Reply to Disputed Findings

For findings you won't address, reply inline with clear reasoning:

```bash
# Reply to a PR comment
gh api repos/{owner}/{repo}/pulls/{pr}/comments/{comment_id}/replies \
  -X POST -f body="<explanation>"
```

**Good reply patterns:**

- **Intentional design:** "This is intentional. [Pattern] is used because [reason]. See [reference]."
- **Already fixed:** "Addressed in commit [hash]. See lines [X-Y]."
- **False positive:** "[Function/pattern] actually does [X], not [Y]. The [context] ensures safety."
- **Acceptable risk:** "This edge case is [acceptable/rare] because [reason]. The cost of fixing outweighs the risk."
- **Separation of concerns:** "This tests [X], not [Y]. [Y] is tested separately in [location]."

### Step 6: Iterate Until Clean

Continue the triage cycle until:
- All blocking errors are fixed
- All check runs pass
- Remaining findings are either fixed or have replies
- No new actionable feedback appears

## SARIF Code Scanning Specifics

SARIF alerts have severity levels:

| Level | Meaning | Action |
|-------|---------|--------|
| `error` / `failure` | Blocking issue | Must fix to merge |
| `warning` | Significant concern | Should fix or explain |
| `note` / `notice` | Observation | Fix if easy, otherwise explain |

**Fetching SARIF annotations:**
```bash
# Find the code scanning check run
CHECK_RUN_ID=$(gh api repos/{owner}/{repo}/commits/{sha}/check-runs \
  --jq '.check_runs[] | select(.name == "anthropic" or .app.slug == "github-code-scanning") | .id' | head -1)

# Get annotations
gh api repos/{owner}/{repo}/check-runs/${CHECK_RUN_ID}/annotations \
  --jq '.[] | {level: .annotation_level, path: .path, line: .start_line, msg: .message}'
```

## Common Dispute Categories

### Clean Architecture / Design Patterns
- "Following clean architecture, domain types are data without behavior. Validation belongs in the use case layer."
- "This is intentional separation of concerns between adapters."

### Premature Optimization
- "This is a micro-optimization for code called [rarely/once]. The [X] dominates runtime."
- "Optimizing this path would be premature - [real bottleneck] is orders of magnitude larger."

### Test Design
- "This tests [specific thing], not [other thing]. Testing both together conflates concerns."
- "The mock isolates [X] for unit testing. Integration tests cover [Y] separately."

### Error Handling
- "Fail-fast is intentional. If [condition], it's a configuration error that should surface immediately."
- "The fallback is documented and logged. Callers can check logs if needed."

### Configuration Design
- "By design, use `[explicit option]` rather than [implicit behavior]. This keeps the API clear."

## Output Format

**IMPORTANT:** You MUST show findings from BOTH sources explicitly. Never skip one.

After triaging, summarize actions taken:

```markdown
## PR Review Triage Summary

### Sources Checked
- [ ] Check Run Annotations (SARIF): X findings from check run ID [id]
- [ ] PR Comments: Y comments from human/bot reviewers

### Findings by Source

#### Check Run Annotations (Static Analysis)
| # | File:Line | Severity | Finding | Decision | Reason |
|---|-----------|----------|---------|----------|--------|
| 1 | path:line | error/warning/note | [description] | accept/dispute/defer | [reason] |

#### PR Comments (Reviewer Feedback)
| # | File:Line | Author | Finding | Decision | Reason |
|---|-----------|--------|---------|----------|--------|
| 1 | path:line | user | [description] | accept/dispute/defer | [reason] |

### Action Summary
| Decision | Count | Details |
|----------|-------|---------|
| Accept (will fix) | X | [brief list] |
| Dispute (won't fix) | Y | [brief list] |
| Defer | Z | [brief list] |
| Duplicate | N | [findings that appear in both sources] |

### Status
- Blocking errors: X fixed, Y remaining
- Check status: [passing/failing]
- Ready for next review: [yes/no]
```

**If either source has 0 findings, explicitly state it** (e.g., "No check run annotations found" or "No PR comments").

## After Loading Context

1. Identify the current PR and HEAD commit SHA
2. **Query BOTH sources** (check this off mentally):
   - Check run annotations (SARIF/static analysis)
   - PR comments (reviewer feedback)
3. Present findings **by source** in separate tables
4. Categorize each finding: accept, dispute, or defer
5. Fix accepted issues
6. Reply to disputed findings on GitHub
7. Report summary with counts from each source
