---
name: golang-unit-test-generator
description: >
  Generates comprehensive, production-quality Go unit tests by analyzing a package's functions and methods, classifying them by test strategy, preparing seed data, and producing table-driven tests with proper mocks and testcontainers. Use this skill whenever the user asks to generate Go unit tests, write tests for a Go package, improve Go test coverage, create Go test files, set up testcontainers for Go integration tests, generate Go test data with faker, or mentions anything related to Go/Golang testing, test generation, or test scaffolding. Even if the user just says "add tests" or "write tests" in a Go project context, use this skill.
---

# Golang Unit Test Generator

You analyze Go packages and generate comprehensive, production-quality unit tests. You reason
carefully before writing any code, following structured analysis and generation phases.

## Input

You receive either a **file path** to a single `.go` file or a **folder path** to a Go package directory.

From this input:
1. Identify all `.go` files (excluding `*_test.go`) in scope
2. Parse all exported and unexported functions, methods, and interfaces
3. Understand the package structure, imports, and dependency graph before proceeding

## Execution Order

Follow this strict sequence — do not skip steps or generate test code before completing classification and data planning:

```
1. Parse input path -> enumerate all .go source files
2. Extract all functions and methods
3. Classify each into Category A / B / C / D / E (Phase 1)
4. Plan data requirements (Phase 2)
5. Generate testdata files / seed scripts if needed (see references/data-generation.md)
6. Generate mock files if needed
7. Generate *_test.go files (Phase 3)
8. Generate TEST_SUMMARY.md
9. Generate TEST_USECASES.md
```

---

## Phase 1: Classify Every Function / Method

Before writing any test, analyze every function and method into one of these categories:

### Category A — Pure Functions
- No external dependencies, no side effects, no I/O
- Input leads to deterministic output only
- **Action:** Write standard table-driven unit tests using `testing.T`. No mocks, no containers.

### Category B — Dependency-Injected Methods
- The struct/method receives dependencies (DB, cache, HTTP client, message broker, etc.) via constructor or interface injection
- **Sub-analysis required:**
  - Can the dependency be instantiated directly in test (e.g., in-memory implementation, simple struct)?
  - Does the test require real infrastructure (PostgreSQL, Redis, Kafka, S3, etc.)?
  - If real infrastructure is required, reassign to **Category D**
  - If a lightweight fake/stub is sufficient, keep as **Category B** and use interface mocks
  - Flag whether **seed data** is required — if yes, plan data generation (Phase 2)

### Category C — Time-Dependent Logic
- Functions that call `time.Now()`, use `time.Sleep()`, compare timestamps, calculate durations, or depend on scheduled behavior
- **Action:**
  - Prefer dependency-injecting a `clock` interface (e.g., `type Clock interface { Now() time.Time }`)
  - If the existing code does not support clock injection and cannot be refactored, emit a WARNING in the test file and in the final summary
  - Document what cannot be reliably tested and why
  - If a workaround exists (e.g., fixed offsets, monkey-patching), apply it with a clear comment

### Category D — Integration Tests with Testcontainers
- Required when the function interacts with a real external service that cannot be meaningfully faked
- Use `testcontainers-go` to spin up real containers
- **Action:**
  - Declare which container image and version to use (e.g., `postgres:15-alpine`, `redis:7`)
  - Write `TestMain` with container lifecycle management (Setup / Teardown)
  - Prepare schema migrations or initialization scripts inside the test setup
  - Prepare and load seed data (Phase 2)
  - All infrastructure must be self-contained — never rely on pre-existing external services

### Category E — Unreachable / Untestable Code
- Dead code paths, `panic`-only branches, auto-generated code, or code requiring OS-level access that cannot be safely tested
- **Action:** Do NOT attempt to cover these. Add a `// NOTE: unreachable -- excluded from coverage` comment and record in the final summary.

---

## Phase 2: Data Preparation

For any test that requires seed data (Categories B, D), you must write and execute a data generation script **before** writing any test code.

Read `references/data-generation.md` for the complete data generation workflow, including:
- Analyzing data requirements (entities, relationships, volumes, constraints)
- Creating the generator script using `github.com/go-faker/faker/v4`
- Executing and verifying the generated data
- Hand-crafting edge case data
- Writing reusable data loading helpers
- Data isolation rules for parallel-safe tests

---

## Phase 3: Test Generation Rules

### General
- Use Go's standard `testing` package as the foundation
- Prefer **table-driven tests** (`[]struct{ name, input, expected }`) for all functions with multiple input variations
- Each test case must have a descriptive `name` field
- Use `t.Run(tc.name, func(t *testing.T) { ... })` for subtests
- Use `t.Parallel()` where safe (pure functions, isolated containers)

### Assertions
- Use `github.com/stretchr/testify/assert` and `require` for assertions
- Use `require.NoError` / `require.Equal` for fatal assertions (stop the test immediately on failure)
- Use `assert.*` for non-fatal checks within the same test case

### Mocks (Category B)
- Prefer interface-based mocks using `mockery v2` conventions
- Place generated mocks in `mocks/` subdirectory
- Clearly document which interface each mock implements
- Set up expectations explicitly; avoid over-mocking

### Testcontainers (Category D)
- Always use `TestMain(m *testing.M)` to manage container lifecycle
- Pass container connection details via package-level variables or a shared `testEnv` struct
- Ensure `defer container.Terminate(ctx)` is always called
- Example structure:
  ```go
  func TestMain(m *testing.M) {
      ctx := context.Background()
      container, connStr, err := setupPostgresContainer(ctx)
      if err != nil { log.Fatal(err) }
      defer container.Terminate(ctx)
      // run migrations, seed data
      os.Exit(m.Run())
  }
  ```

### Coverage
- Target the highest achievable coverage for all reachable code paths
- Cover: happy path, error paths, boundary values, empty/nil inputs, concurrent access (if applicable)
- Do NOT artificially inflate coverage by testing unreachable branches — note them instead

### File Naming
- Place test files alongside source files: `foo.go` -> `foo_test.go`
- Place testdata files in `<package_dir>/testdata/`
- Place mocks in `<package_dir>/mocks/`

---

## Phase 4: Post-Generation Deliverables

After all test files have been generated, produce exactly two summary documents:

### TEST_SUMMARY.md

```markdown
## Completed
List every function/method that has been tested, with:
- Test file location
- Test category (A/B/C/D)
- Number of test cases written
- Estimated coverage contribution

## Warnings & Special Notes
- All time-dependent functions that could not be fully tested
- Any assumptions made about data or environment
- Mock interfaces that may drift from real implementations
- Tests that are flaky by nature (timing, ordering)

## Not Covered / Incomplete
- Unreachable code paths (with reason)
- Functions skipped due to missing context or excessive complexity
- Items requiring manual intervention before tests can run

## Prerequisites & Setup Instructions
- Required Go modules to add (go get ...)
- Docker requirement for Testcontainers tests
- Any environment variables needed
- How to run the full test suite (go test ./... -v -race)
```

### TEST_USECASES.md

A structured list of every test case written, organized by function:

```markdown
## <PackageName>.<FunctionOrMethodName>

| # | Test Case Name | Input Summary | Expected Outcome | Category |
|---|----------------|---------------|------------------|----------|
| 1 | valid input     | ...           | returns X        | A        |
| 2 | nil pointer     | nil           | returns error    | A        |
| 3 | DB unavailable  | valid input   | returns ErrDB    | D        |
```

---

## Constraints & Guardrails

- **Do not modify source files.** Only create `*_test.go` files and supporting assets.
- **Do not introduce new production dependencies.** Test-only dependencies in `go.mod` are acceptable.
- **Do not use `reflect.DeepEqual` directly** — use `testify` instead.
- **Do not hardcode credentials or ports** — use dynamic port allocation from Testcontainers.
- **Always check `err != nil`** before asserting on returned values.
- If a function is not exported and not testable via exported API, use the `package foo` (white-box) test file pattern, not `package foo_test`.
