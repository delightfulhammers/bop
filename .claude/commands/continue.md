# Continue Work

Figure out where we left off and resume, with awareness of the Phase 3 plan.

## 1. Check Planning State First

**Read the design documents to understand current priorities:**

```
# Check Phase 3 design docs
ls docs/design/
head -50 docs/design/04-ROADMAP-PHASE-3.1.md
```

## 2. Gather Git Context

1. **Check current branch:**
   ```
   git branch --show-current
   git status
   ```

2. **Look for issue context in branch name:**
   - Branch names like `feature/42-...` or `fix/123-...` indicate an issue number
   - If found, fetch that issue: `gh issue view <number>`

3. **Check for uncommitted work:**
   ```
   git status
   git stash list
   ```

4. **Check recent commits on this branch:**
   ```
   git log --oneline -10
   ```

5. **Check for open PRs I might be working on:**
   ```
   gh pr list --state open --author @me
   gh pr list --state open
   ```

## 3. Check Phase 3 Issue Status

```
# See high-priority Phase 3 issues
gh issue list --state open --label "phase:3" --label "priority:high" --limit 10

# See all Phase 3 issues
gh issue list --state open --label "phase:3" --limit 20

# See issues without phase label (may need triage)
gh issue list --state open --limit 10
```

**Phase 3 Priority Order (from design docs):**

### Phase 3.1: Triage Automation (P0)
1. MCP Server skeleton (`cmd/bop-mcp/`)
2. Domain types (`internal/domain/triage.go`)
3. Triage service (`internal/usecase/triage/`)
4. MCP tool handlers (9 tools)
5. Claude Code skill update
6. `bop triage` CLI (P2)

### Phase 3.2: Reviewer Personas (P0)
1. Reviewer configuration schema
2. Persona prompt builder
3. Orchestrator updates
4. Merger role awareness

### Phase 3.3: Dynamic Model Selection (P1)
1. Model selector interface
2. Token-tier routing
3. Change-type routing

## 4. Present Findings

Summarize:
- Current branch and its likely associated issue
- Any uncommitted or stashed changes
- Open PRs that might need attention
- Best guess at what was being worked on
- **Current Phase 3.1 priorities** based on roadmap
- **Any recently closed issues** that may need follow-up

## 5. Suggest Next Steps

Based on the context:

1. **If on a feature branch with uncommitted work:**
   "It looks like you were working on #XXX. Should I continue?"

2. **If on main with clean state:**
   "You're on main with no pending work. Based on Phase 3.1 roadmap, I suggest:
   - Start with: [next milestone task from 04-ROADMAP-PHASE-3.1.md]
   Which would you like to work on?"

3. **If there's an open PR:**
   "There's an open PR for #XXX. Should I check its status and address any feedback?"

## 6. Before Starting Work

Do NOT start implementation until the user confirms.

When they confirm:
1. If starting a new issue, create feature branch: `git checkout -b feature/<issue-number>-short-description`
2. Read the relevant design doc section
3. Begin implementation following TDD workflow

## 7. Remember the Definition of Done

Before marking any work as complete:

- [ ] Tests written (TDD)
- [ ] Code formatted (`gofmt -w .`)
- [ ] All tests pass (`go test ./...`)
- [ ] Build succeeds (`go build -o bop ./cmd/bop`)
- [ ] No race conditions (`go test -race ./...`)

## 8. Key Design Documents

When working on Phase 3 features, reference:
- `docs/design/01-PRD.md` - Requirements and user personas
- `docs/design/02-ARCHITECTURE.md` - System architecture and component design
- `docs/design/03-TDD-TRIAGE-MCP-SERVER.md` - MCP server implementation details
- `docs/design/04-ROADMAP-PHASE-3.1.md` - Task breakdown and milestones
