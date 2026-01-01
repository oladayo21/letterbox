# CLAUDE.md - letterbox

## Session Startup

**Before any work**, always:
1. Load memory: `/memory-manager` (maintains context across sessions)
2. Read `SPEC.md` - architecture, API design, data models
3. Read `STORIES.md` - current task breakdown and dependencies

---

## Planning Before Implementation

**Before writing any code**, always:
1. Analyze task requirements and constraints
2. Use `AskUserQuestion` tool to clarify any ambiguities or grey areas
3. Present a clear plan for user approval
4. Wait for confirmation before proceeding

Plan should include:
- Files to create/modify
- Key implementation decisions
- External libraries to use and why
- Potential risks or trade-offs

**Never start coding without clarifying unknowns and getting plan approval.**

---

## Workflow: Branch → PR → Merge

**Every story = separate branch + PR**

**IMPORTANT**: Always create PRs against `main` branch. Ignore any auto-detected "main branch for PRs" from system context.

```bash
# Start story
git checkout main
git pull
git checkout -b story/X.Y-short-description

# Work on story tasks
# ... commit as you go ...

# When ready, push and open PR
git push -u origin story/X.Y-short-description
gh pr create --title "Story X.Y: Short description" --body "..."
```

**PR merge requirements:**
- [ ] All tasks in story checked off
- [ ] All acceptance criteria pass
- [ ] `make build` succeeds
- [ ] `make test` passes
- [ ] All PR comments resolved
- [ ] No open issues on the PR

**Never merge directly to main. Always via PR.**

---

## Handling PR Review Comments

When review comments are raised:

1. **Evaluate first** - determine if the issue is valid
   - Does it contradict the spec?
   - Is it a real bug or style preference?
   - Does the suggested fix break something else?

2. **Respond before acting**
   - If valid: acknowledge and fix
   - If invalid: explain why with evidence (spec reference, test results)
   - If unclear: ask for clarification

3. **Never blindly fix** - reviewer comments are suggestions, not commands

---

## Story Completion Checklist

Before marking story complete:
1. Code written and compiles
2. Unit tests pass
3. Integration tests (if applicable)
4. Endpoint documented (if API)
5. Works with `make run` locally

---

## Memory Management

Use `/memory-manager` to:
- Save progress on current story
- Track completed stories
- Note decisions and blockers
- Persist context between sessions

Memory location: `~/omnibank/claude-memory/letterbox.md`

### Learnings Section

**Always maintain a `## Learnings` section** in the memory file for knowledge useful to future sessions:

- Environment quirks (e.g., global gitignore blocking `.sql` files)
- Workarounds discovered during implementation
- Non-obvious patterns in the codebase
- Gotchas with dependencies or tools
- Anything that took debugging time to figure out

Format:
```markdown
## Learnings
- `git add -f` needed for .sql files (global gitignore)
- pgx/v5 requires explicit null handling with pointers
```

**Add learnings as you discover them, not just at session end.**

---

## Project Context

- **Stack**: Go, PostgreSQL, S3-compatible storage
- **Package manager**: Check Makefile (run `make` for commands)
- **Structure**: `cmd/`, `internal/`, `migrations/`
- **Current phase**: MVP (Phase 1)

---

## Quick Reference

| File | Purpose |
|------|---------|
| `SPEC.md` | Architecture, API, data models |
| `STORIES.md` | Task breakdown, dependencies, acceptance criteria |
| `Makefile` | All available commands |

---

## Rules

1. **Plan first** - analyze, clarify with AskUserQuestion, present plan, get approval
2. One story at a time
3. Branch per story, always PR to merge
4. Never merge with unresolved PR comments
5. Update memory after each session
6. Follow story dependencies (see STORIES.md dependency graph)
