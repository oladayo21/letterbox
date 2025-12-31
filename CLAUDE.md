# CLAUDE.md - letterbox

## Session Startup

**Before any work**, always:
1. Load memory: `/memory-manager` (maintains context across sessions)
2. Read `SPEC.md` - architecture, API design, data models
3. Read `STORIES.md` - current task breakdown and dependencies

---

## Workflow: Branch-per-Story

**Every story = separate branch**

```bash
# Start story
git checkout main
git pull
git checkout -b story/X.Y-short-description

# Work on story tasks
# ...

# Only merge when ALL acceptance criteria met
git checkout main
git merge story/X.Y-short-description
```

**Never merge to main until:**
- [ ] All tasks in story checked off
- [ ] All acceptance criteria pass
- [ ] `make build` succeeds
- [ ] `make test` passes
- [ ] Code compiles without errors

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

1. One story at a time
2. Branch per story
3. No partial merges
4. Update memory after each session
5. Follow story dependencies (see STORIES.md dependency graph)
