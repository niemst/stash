# AGENTS.md

> Rules for any AI agent working in this repository.

Read this file **before every task**. These rules override any default behavior you might have. If a task file contradicts a rule here, the rule wins — unless the task explicitly overrides a named rule by number.

---

## 1. The golden rules

**1.1 When in doubt, ask. Do not guess.**
Ambiguity in a spec is a signal, not an invitation to improvise. If the task file is unclear, stop and ask before writing code. Silent assumptions are bugs waiting to happen.

**1.2 Do less.**
Every line of code, every type, every method, every file must justify its existence. The smallest thing that solves the problem wins. If you're about to add "while I'm here...", stop and put it in `TODO.md` instead.

**1.3 No surprises.**
Do only what the task asks. Do not add features, abstractions, refactors, or "improvements" the task did not request. If you see something worth fixing elsewhere, note it — do not fix it.

**1.4 Read before writing.**
Before editing a file, read it. Before creating a file, check if something similar exists. Before introducing a dependency, check if one already serves the purpose.

**1.5 Honesty over completeness.**
If you couldn't implement something, say so. If a test is failing, say so. If you skipped a requirement, say so. Do not fake completeness. Do not claim tests pass without running them.

---

## 2. Scope discipline

**2.1 Stay inside the task.**
If the task is "implement `pkg/store/crud.go`", do not touch `pkg/store/search.go` unless explicitly required. Changes outside the declared scope are rejected on review.

**2.2 No drive-by refactors.**
You may notice code elsewhere that is ugly, duplicated, or suboptimal. Do not refactor it. Note it in `TODO.md` with a one-line description.

**2.3 No unrelated dependency upgrades.**
Do not bump `go.mod` versions unless the task requires it.

**2.4 No formatting-only commits in feature work.**
If you reformat unrelated code because your editor does it automatically, revert those changes before committing.

---

## 3. Architectural rules (non-negotiable)

**3.1 One-way dependencies.**
Packages under `pkg/` are leaf packages and must not import siblings.
Packages under `internal/` may depend on sibling `internal/` packages (e.g., `memory` depends on `store`). Dependencies must always flow downward: `cmd` → `internal/memory` → `internal/store`.

**3.2 Lower layers know nothing about higher layers.**
`internal/store` does not know about `internal/memory`. `internal/memory` does not know about the kernel. Domain concepts (Fact, Entity, Episode, Memory, Kernel) never appear in lower layers' code, comments, errors, logs, or schema.

**3.3 No global state.**
No package-level mutable variables. No singletons. No init functions that do work. If something needs state, it lives in a struct the caller constructs.

**3.4 No background goroutines the caller didn't start.**
A package must not spawn goroutines during construction. If background work is needed, expose a method the caller explicitly calls to start it.

**3.5 Return errors, do not log them.**
Libraries do not log. Libraries return errors. The caller decides what to do with them. The only exception is explicit debug tracing gated behind an opt-in flag.

**3.6 No `panic` for runtime errors.**
Panics are for programmer errors (nil dereferences, impossible states). Runtime errors — bad input, failed I/O, missing data — return errors.

**3.7 Store everything in metadata.**
Higher layers store domain data in `Record.Metadata`, not in new tables or schemas.
Example: `memory` stores events as `Record` with `metadata.type="event"`.
This keeps store generic and avoids interface bloat.

**3.8 Compose, don't extend interfaces.**
If a package needs new capabilities, create a new tool that composes with existing ones.
Don't add methods to stable interfaces. Example: Need graph relationships? Create `internal/graph` that uses `internal/store`, don't add graph methods to `Store` interface.

**3.9 Higher layers are storage-agnostic.**
Packages like `internal/memory` contain pure business logic only.
No SQL, no schema management, no database-specific code.
Use the abstraction (`store.Store`), don't implement storage.

---

## 4. Go code rules

**4.1 Idiomatic Go only.**
Follow `gofmt`, `go vet`, and `staticcheck`. Errors end with context: `fmt.Errorf("operation: %w", err)`. Accept interfaces, return structs. Small interfaces. No premature generics.

**4.2 Prefer concrete types over `any`.**
Reserve `any` for genuinely untyped data (like JSON metadata values). Do not use `any` to avoid declaring a type.

**4.3 No CGO unless explicitly required.**
Pure Go. Single static binary is the default.

**4.4 Standard library first.**
Before adding a dependency, check if stdlib handles it. For HTTP, `net/http` usually suffices. For JSON, `encoding/json`. For SQL, `database/sql` or `pgx`. Avoid framework-style dependencies.

**4.5 Error types.**
Use sentinel errors (`var ErrX = errors.New(...)`) checked with `errors.Is`. Use typed errors (`type XError struct{...}`) checked with `errors.As` only when the caller needs structured fields. Never return bare strings as errors.

**4.6 Context is the first parameter.**
Every function that does I/O, blocks, or could be long-running takes `ctx context.Context` as its first parameter. Respect cancellation.

**4.7 No naked returns in functions longer than 5 lines.**
Explicit returns. Readability over cleverness.

**4.8 Test files co-located with code.**
`foo.go` → `foo_test.go` in the same package. Integration tests that need external services go in a separate `_test` package with a build tag if they're slow.

---

## 5. Naming rules

**5.1 Honest names.**
Call things what they are. `Store` stores. `Record` is a record. No marketing words ("smart", "magic", "intelligent") in code. No aspirational names that don't match what the thing does today.

**5.2 No abbreviations unless universal.**
`context` not `ctx` in comments; `ctx` in code is fine because it's universal. `req`, `res`, `err`, `db` are fine. `usrMgr`, `prodCfg` are not.

**5.3 Package names are single lowercase words.**
No `store_utils`, no `storehelper`. If you need it, it's probably in the wrong place.

**5.4 Exported symbols have godoc comments.**
Every exported type, function, method, constant, and variable. The comment starts with the symbol name and describes what it does, not how.

---

## 6. Testing rules

**6.1 Every new exported behavior has a test.**
No exceptions. If you can't write a test for it, you don't understand it yet.

**6.2 Tests test behavior, not implementation.**
Do not assert on private fields. Do not mock what you don't own. Test through the public API.

**6.3 No flaky tests.**
Tests that pass "usually" are worse than no tests. Use deterministic inputs. Avoid time-based assertions without `time.Now` injection. Avoid network calls without test containers.

**6.4 Integration tests use real dependencies.**
For Postgres, use `testcontainers-go`. Do not write hand-rolled mocks of databases — they lie.

**6.5 Table-driven tests where natural.**
Use Go's table-driven pattern for testing multiple inputs to the same function. Do not force it where it makes things unclear.

**6.6 One assertion per test when possible.**
A test that checks one thing tells you exactly what broke. A test with ten assertions tells you something broke.

---

## 7. Documentation rules

**7.1 README first.**
Every package under `pkg/` has a `README.md`. It explains what the package is, what it isn't, and how to use it. Write it before writing the implementation.

**7.2 Example code must run.**
Code in READMEs and godoc examples must compile and execute without modification. If it's a snippet, mark it as such.

**7.3 No lies in docs.**
If the code changes and the docs don't, the docs are a bug. Update them in the same commit.

**7.4 Write for the reader who comes in cold.**
Assume the reader knows Go but doesn't know this project. Explain *why*, not just *what*.

---

## 8. Commit and PR rules

**8.1 One logical change per commit.**
A commit does one thing: implement X, fix Y, rename Z. Not all three.

**8.2 Commit messages follow conventional format.**
`feat(pkg): add Put and Get`, `fix(store): handle nil metadata`, `docs(readme): add usage example`, `test(postgres): cover edge cases`. Subject line ≤72 chars. Body wraps at 80.

**8.3 Reference task files in commits.**
When working from a task file, include it: `feat(store): implement predicate AST [task: 0001]`.

**8.4 Never force-push to shared branches.**
`main` and feature branches others might be reviewing are off-limits for force-push.

**8.5 Never commit secrets, credentials, or `.env` files.**
If you see one committed, remove it and notify the human.

---

## 9. What to never do

- ❌ Never invent file paths or function names. If you don't know, ask or search the repo.
- ❌ Never claim a test passes without running it.
- ❌ Never suppress errors with `_ = err` without a comment explaining why.
- ❌ Never add TODO comments in shipped code — put them in `TODO.md`.
- ❌ Never write code you can't explain.
- ❌ Never copy code from the web without verifying it and its license.
- ❌ Never disable lints without a comment explaining why.
- ❌ Never use `fmt.Println` for debug output in shipped code.
- ❌ Never add "helper" packages like `utils`, `common`, `misc`. Put the helper where it belongs.
- ❌ Never introduce a new dependency without declaring it in your response for human review.

---

## 10. Working style

**10.1 Think, then write.**
For any non-trivial task, write a short plan before writing code. State: what you will change, what files you will touch, what you will not touch, what could go wrong.

**10.2 Small steps, verified.**
Make a change. Run the tests. Commit. Repeat. Do not write 500 lines before the first test run.

**10.3 Prefer questions over assumptions.**
If the task file says "implement the store interface" and you don't see the interface yet, ask where it is. Don't invent it.

**10.4 Surface blockers early.**
If you hit a blocker that will take more than 10 minutes of investigation, stop and report it. Do not silently work around it.

**10.5 Leave the repo cleaner than you found it.**
Within your scope. Remove dead imports. Fix typos you introduced. Do not expand scope to "clean up" elsewhere.

---

## 11. Project-specific conventions

**11.1 Directory layout.**
```
repo/
├── docs/tasks/         ← task specs; read the relevant one before working
├── cmd/                ← binary entry points (CLI, Server, Agent)
├── internal/           ← product-specific code (primary workspace)
│   ├── store/          ← storage engine (generic but internal)
│   └── memory/         ← memory layer (uses store)
└── pkg/                ← (optional) reusable libraries extracted for external use
```

**11.2 Task files.**
Task files live in `docs/tasks/NNNN-name.md`. They are the source of truth for scope. If the task file contradicts this AGENTS.md, follow AGENTS.md and ask.

**11.3 Definition of done.**
Every task declares its own definition of done. You are not done until every item is checked, tests pass, and the code compiles clean (`go vet`, `staticcheck`).

**11.4 No premature optimization.**
Write correct code first. Benchmark before optimizing. If you think something might be slow, write a benchmark to prove it before changing the code.

**11.5 Product focus.**
This repository builds a standalone product (CLI/Server/Agent). `internal/` is the primary workspace. Code is moved to `pkg/` only when explicitly intended for external reuse by other projects.

**11.6 Unix philosophy for tool composition.**
Packages should follow Unix philosophy: do one thing well, compose with other tools.
- `internal/store` is the filesystem primitive
- `internal/memory` is like `awk`/`sed` for intelligence  
- Future tools (`graph`, `index`, `cache`) compose with store
Tools use lower layers, don't extend them. Compose, don't modify interfaces.

---

## 12. When these rules conflict

If a rule here conflicts with a task file: **follow AGENTS.md and ask the human.**
If two rules here conflict: **ask the human.**
If you think a rule is wrong: **follow it and flag it for discussion after the task.**

Do not silently decide a rule doesn't apply.

---

## 13. The meta-rule

> **If you're not sure whether something is a good idea, it isn't.**

When your instinct says "this is clever," pause. Clever code is unmaintainable code. Boring code ships.