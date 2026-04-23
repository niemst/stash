# Task: Phase 2 CLI Commands

**Status:** Completed  
**Date:** 2026-04-23
**Completed:** 2026-04-23

---

## 1. Context

**Goal:** Expose Phase 2 cognitive processes (consolidation, contradiction detection, reflection, reinforcement) via CLI commands. Wrap the library layer with user-friendly, immediate-mode interfaces.

**Why:** Phase 2 library methods exist (`ConsolidateRecent`, `FindContradictions`, `Reflect`, `Reinforce`) but are only accessible programmatically. CLI commands democratize access — users can run introspection, synthesis, and fact strengthening without writing Go code.

**What this is:**
- Four new subcommands under `facts` group: `consolidate`, `contradictions`, `reflect`, `reinforce`
- CLI flags mirror library method parameters (timewindow, limit, entity, property, value, namespace)
- JSON output for machine readability, human-friendly summaries for inspection
- Integration with existing `cmd/cli/` structure and bootstrap context

**What this is NOT:**
- Auto-triggering of commands (manual user invocation only)
- Caching or optimization of repeated runs
- Advanced filtering or query DSLs (flat flags only)
- Daemon mode or scheduled tasks
- Web API (Phase 4+)

---

## 2. Boundaries

**In scope:**
- `cmd/cli/facts_consolidate.go` — `stash facts consolidate [--namespace=X] [--window=1h]`
- `cmd/cli/facts_contradictions.go` — `stash facts contradictions [--namespace=X]`
- `cmd/cli/facts_reflect.go` — `stash facts reflect [--namespace=X]`
- `cmd/cli/facts_reinforce.go` — `stash facts reinforce --entity=X --property=Y --value=Z [--count=1]`
- Update `cmd/cli/main.go` to register new `facts` command group
- All commands use JSON output format (parseable, consistent with existing `events` commands)
- Basic error handling and validation (required flags, invalid values)

**Not in scope:**
- Format flags (`--format=text` vs `--format=json`) — JSON only
- Batch operations or piping (call CLI multiple times)
- Advanced query syntax or templates
- Shell completion scripts
- Man pages or help expansion
- Performance optimizations (reuse bootstrap context via existing pattern)

---

## 3. Approach & Review

**Command Design:**

```bash
# Consolidate recent events into facts
stash facts consolidate [--namespace=ns] [--window=1h] [--limit=20]

# Find contradictions in facts
stash facts contradictions [--namespace=ns]

# Introspect memory state
stash facts reflect [--namespace=ns]

# Strengthen facts by observation count
stash facts reinforce --entity=alice --property=location --value=paris [--count=1]
```

**Output Format:**

All commands return JSON. Example structures:
- `consolidate` → `{facts: [...], synthesized_count: N, event_count: N}`
- `contradictions` → `{contradictions: [...], count: N}`
- `reflect` → full `ReflectionReport` as JSON
- `reinforce` → `{reinforced: true, fact_id: "...", confidence: 0.71}`

**Verification Plan:**

1. Manual CLI invocation for each command with sample data
2. Edge case handling: missing required flags, empty result sets, invalid entity specs
3. Integration with bootstrap context (reuse existing pattern)
4. JSON output validated as parseable

**Acceptance Criteria:**

- [ ] `stash facts consolidate` accepts `--namespace`, `--window`, `--limit` flags and outputs JSON
- [ ] `stash facts contradictions` accepts `--namespace` flag and outputs JSON list
- [ ] `stash facts reflect` accepts `--namespace` flag and outputs full ReflectionReport as JSON
- [ ] `stash facts reinforce` requires `--entity`, `--property`, `--value` and accepts `--count`; outputs JSON with fact ID and new confidence
- [ ] All commands integrate via bootstrap context without duplication
- [ ] CLI help text is clear (usage strings in main.go)
- [ ] No new dependencies introduced
- [ ] All commands error gracefully on invalid input

---

## 4. Implementation Notes

**File Structure:**
- `cmd/cli/facts_consolidate.go` — consolidateCmd function (~50 lines)
- `cmd/cli/facts_contradictions.go` — contradictionsCmd function (~40 lines)
- `cmd/cli/facts_reflect.go` — reflectCmd function (~30 lines)
- `cmd/cli/facts_reinforce.go` — reinforceCmd function (~50 lines)
- Update `cmd/cli/main.go` — add facts command group with 4 subcommands

**Bootstrap Pattern (Reuse Existing):**
```go
bc, ok := cmd.Root().Metadata["bootstrapCtx"].(*bootstrap.Context)
if !ok {
  return fmt.Errorf("bootstrap context not available")
}
// Use bc.Memory, bc.Logger, etc.
```

**Error Messages:**
- Required flag missing: `"--entity flag is required"`
- Invalid value: `"--window must be a valid duration (e.g. 1h, 30m)"`
- Empty result: Print empty JSON array, not error (it's a valid result)

---

## 5. Acceptance Criteria (Testable)

- [x] Four commands implemented and wired into CLI
- [x] All commands reachable via `stash facts <subcommand>`
- [x] JSON output parseable for all commands
- [x] Required flags validated (error if missing)
- [x] Edge cases handled (empty namespaces, no facts, no contradictions)
- [x] Bootstrap context correctly extracted and used
- [x] No code duplication; shared patterns from Phase 1 CLI

---

## 6. Files Affected

- `cmd/cli/facts_consolidate.go` — new
- `cmd/cli/facts_contradictions.go` — new
- `cmd/cli/facts_reflect.go` — new
- `cmd/cli/facts_reinforce.go` — new
- `cmd/cli/main.go` — modified (add facts command group)

---

## 7. Execution Log

- [2026-04-23 23:50] Created facts_consolidate.go (consolidateCmd)
- [2026-04-23 23:50] Created facts_contradictions.go (contradictionsCmd)
- [2026-04-23 23:50] Created facts_reflect.go (reflectCmd)
- [2026-04-23 23:50] Created facts_reinforce.go (reinforceCmd)
- [2026-04-23 23:50] Updated main.go with facts command group
- [2026-04-23 23:50] Tested all commands with sample data
- [2026-04-23 23:50] Committed changes

---

## 8. Outcome

**Final Result:**

Phase 2 cognitive processes are now exposed via CLI. Users can:
- Synthesize events into facts via `consolidate`
- Detect conflicts via `contradictions`
- Introspect memory via `reflect`
- Strengthen facts via `reinforce`

All commands integrate seamlessly with existing bootstrap infrastructure and output machine-readable JSON.

**What Changed:**
- Added 4 new CLI command files + updated main.go registration
- ~170 lines of CLI glue code (no new logic, only argument parsing and output formatting)
- Zero dependencies, zero breaking changes

**What Was Verified:**
- All 4 commands callable and reachable
- Output formats match expectations
- Bootstrap context correctly threaded
- Error handling for missing flags and edge cases
- JSON parsing validates

**What Remains Open:**
- Advanced CLI features (batch, format options, shell completion) deferred to future
- Performance optimization (command startup time) not measured
