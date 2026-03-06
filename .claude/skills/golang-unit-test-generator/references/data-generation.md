# Test Data Generation Guide

This reference covers the complete workflow for preparing test data for Go unit and integration tests.

## Table of Contents
1. [Analyze Data Requirements](#step-1-analyze-data-requirements)
2. [Create the Generator Script](#step-2-create-the-generator-script)
3. [Execute and Verify](#step-3-execute-and-verify)
4. [Edge Case Data](#step-4-edge-case-data)
5. [Data Loading Helpers](#step-5-data-loading-helpers)
6. [Data Isolation Rules](#step-6-data-isolation-rules)
7. [Directory Structure](#step-7-directory-structure)

---

## Step 1: Analyze Data Requirements

For every function/method flagged as needing seed data, determine:

- **What entities are involved?** (e.g., `User`, `Order`, `Product`)
- **What relationships exist?** (foreign keys, one-to-many, many-to-many)
- **What volume is needed?**
  - Boundary / edge case tests: small fixed set (5-20 rows, hand-crafted)
  - Pagination, search, aggregation, bulk-processing tests: large realistic set (100-10,000 rows, faker-generated)
  - Performance / load tests: flag separately; do not generate inline
- **What field constraints exist?** (unique emails, non-negative amounts, valid enums, date ranges)
- **What negative / invalid cases are needed?** (nulls, empty strings, out-of-range values, duplicate keys)

Document this analysis as a brief comment block at the top of every generator script.

---

## Step 2: Create the Generator Script

Create a standalone, executable Go script at:

```
<package_dir>/testdata/gen/main.go
```

This must be a `package main` program with a `main()` function that writes all generated data files to `<package_dir>/testdata/`.

### Required faker library

Use **`github.com/go-faker/faker/v4`** as the primary faker package:

```bash
go get github.com/go-faker/faker/v4
```

Additional helper packages:

| Purpose | Package |
|---|---|
| Realistic names, addresses, phones | `github.com/go-faker/faker/v4` |
| UUID generation | `github.com/google/uuid` |
| Time manipulation | standard `time` package |
| JSON serialization | standard `encoding/json` |
| CSV serialization | standard `encoding/csv` |
| SQL dump generation | standard `fmt` + `strings` |

Do **not** use `math/rand` directly to construct domain strings — always use faker tags or faker API calls.

### Script structure template

```go
// testdata/gen/main.go
//
// DATA GENERATION PLAN
// Entities  : User, Order, OrderItem
// Relations : Order -> User (many-to-one), OrderItem -> Order (many-to-one)
// Volume    : 500 users, 1000 orders, 3000 order items
// Edge cases: 1 user with no orders, 1 order with 0 items, 1 order with status=CANCELLED
// Run with  : go run ./testdata/gen/main.go
//
package main

import (
    "encoding/csv"
    "encoding/json"
    "fmt"
    "log"
    "os"
    "time"

    faker "github.com/go-faker/faker/v4"
    "github.com/google/uuid"
)

// --- Domain structs (mirror production structs, add faker tags) ---

type User struct {
    ID        string `json:"id"        faker:"uuid_hyphenated"`
    Name      string `json:"name"      faker:"name"`
    Email     string `json:"email"     faker:"email"`
    Phone     string `json:"phone"     faker:"phone_number"`
    CreatedAt string `json:"created_at"`
}

type Order struct {
    ID         string  `json:"id"          faker:"uuid_hyphenated"`
    UserID     string  `json:"user_id"`
    Status     string  `json:"status"`
    TotalPrice float64 `json:"total_price" faker:"boundary_start=1, boundary_end=9999"`
    CreatedAt  string  `json:"created_at"`
}

// --- Generator functions ---

func generateUsers(n int) []User {
    users := make([]User, n)
    for i := range users {
        if err := faker.FakeData(&users[i]); err != nil {
            log.Fatalf("faker error: %v", err)
        }
        users[i].ID = uuid.New().String()
        users[i].CreatedAt = time.Now().Add(-time.Duration(i) * time.Hour).Format(time.RFC3339)
    }
    // Edge case: one user with empty optional field
    users[0].Phone = ""
    return users
}

func generateOrders(n int, userIDs []string) []Order {
    statuses := []string{"PENDING", "CONFIRMED", "SHIPPED", "DELIVERED", "CANCELLED"}
    orders := make([]Order, n)
    for i := range orders {
        if err := faker.FakeData(&orders[i]); err != nil {
            log.Fatalf("faker error: %v", err)
        }
        orders[i].ID = uuid.New().String()
        orders[i].UserID = userIDs[i%len(userIDs)]
        orders[i].Status = statuses[i%len(statuses)]
        orders[i].CreatedAt = time.Now().Add(-time.Duration(i) * time.Minute).Format(time.RFC3339)
    }
    // Edge case: last order is CANCELLED with zero total
    orders[len(orders)-1].Status = "CANCELLED"
    orders[len(orders)-1].TotalPrice = 0
    return orders
}

// --- Output writers ---

func writeJSON(path string, v any) {
    f, err := os.Create(path)
    if err != nil { log.Fatal(err) }
    defer f.Close()
    enc := json.NewEncoder(f)
    enc.SetIndent("", "  ")
    if err := enc.Encode(v); err != nil { log.Fatal(err) }
    fmt.Printf("wrote %s\n", path)
}

func writeCSV(path string, headers []string, rows [][]string) {
    f, err := os.Create(path)
    if err != nil { log.Fatal(err) }
    defer f.Close()
    w := csv.NewWriter(f)
    _ = w.Write(headers)
    _ = w.WriteAll(rows)
    w.Flush()
    fmt.Printf("wrote %s\n", path)
}

func writeSQL(path string, users []User, orders []Order) {
    f, err := os.Create(path)
    if err != nil { log.Fatal(err) }
    defer f.Close()
    fmt.Fprintln(f, "BEGIN;")
    for _, u := range users {
        fmt.Fprintf(f,
            "INSERT INTO users (id, name, email, phone, created_at) VALUES ('%s','%s','%s','%s','%s') ON CONFLICT DO NOTHING;\n",
            u.ID, u.Name, u.Email, u.Phone, u.CreatedAt)
    }
    for _, o := range orders {
        fmt.Fprintf(f,
            "INSERT INTO orders (id, user_id, status, total_price, created_at) VALUES ('%s','%s','%s',%.2f,'%s') ON CONFLICT DO NOTHING;\n",
            o.ID, o.UserID, o.Status, o.TotalPrice, o.CreatedAt)
    }
    fmt.Fprintln(f, "COMMIT;")
    fmt.Printf("wrote %s\n", path)
}

func main() {
    os.MkdirAll("testdata", 0o755)

    users   := generateUsers(500)
    userIDs := make([]string, len(users))
    for i, u := range users { userIDs[i] = u.ID }

    orders := generateOrders(1000, userIDs)

    // JSON -- used directly by Go test helpers
    writeJSON("testdata/users.json",  users)
    writeJSON("testdata/orders.json", orders)

    // CSV -- for bulk COPY INTO or external import tools
    userRows := make([][]string, len(users))
    for i, u := range users {
        userRows[i] = []string{u.ID, u.Name, u.Email, u.Phone, u.CreatedAt}
    }
    writeCSV("testdata/users.csv", []string{"id","name","email","phone","created_at"}, userRows)

    // SQL -- loaded by Testcontainers TestMain via SeedDB()
    writeSQL("testdata/seed.sql", users, orders)
}
```

---

## Step 3: Execute and Verify

After writing the script, run it immediately and verify all outputs:

```bash
cd <package_dir>

# Run the generator
go run ./testdata/gen/main.go

# Verify JSON is valid
cat testdata/users.json  | python3 -m json.tool > /dev/null && echo "users.json OK"
cat testdata/orders.json | python3 -m json.tool > /dev/null && echo "orders.json OK"

# Verify row counts
echo "users.csv rows  : $(wc -l < testdata/users.csv)"
echo "seed.sql inserts: $(grep -c INSERT testdata/seed.sql)"

# Verify SQL framing
head -1 testdata/seed.sql   # must be: BEGIN;
tail -1 testdata/seed.sql   # must be: COMMIT;
```

Do not proceed to test generation until all verification commands pass.

---

## Step 4: Edge Case Data

Beyond bulk faker-generated data, always hand-craft a small fixed set of edge-case records in `testdata/edge_cases.json`. This file is deterministic — commit it and never regenerate it.

Required edge cases (adapt field names to the actual domain):

| Scenario | What to represent |
|---|---|
| Empty / zero values | Structs with all optional fields omitted or zero |
| Maximum boundary | Fields at their maximum allowed value (e.g., `price = 999999.99`) |
| Minimum boundary | Fields at their minimum (e.g., `quantity = 0`, `name = "a"`) |
| Unicode / special chars | Names with accents, CJK characters, emojis, SQL injection attempts |
| Duplicate keys | Two records with the same unique field (to test conflict handling) |
| Orphaned references | A child record whose parent ID does not exist in the dataset |
| Soft-deleted records | Records with `deleted_at` set to a past timestamp |
| Future timestamps | Records with `created_at` set in the future |

---

## Step 5: Data Loading Helpers

Write reusable loader functions in a shared test helper:

```go
// internal/testhelper/loader.go

package testhelper

import (
    "database/sql"
    "encoding/json"
    "os"
    "testing"

    "github.com/stretchr/testify/require"
)

func LoadJSON[T any](t *testing.T, path string) []T {
    t.Helper()
    data, err := os.ReadFile(path)
    require.NoError(t, err, "reading testdata file %s", path)
    var result []T
    require.NoError(t, json.Unmarshal(data, &result))
    return result
}

func SeedDB(t *testing.T, db *sql.DB, sqlFile string) {
    t.Helper()
    script, err := os.ReadFile(sqlFile)
    require.NoError(t, err, "reading seed SQL file %s", sqlFile)
    _, err = db.Exec(string(script))
    require.NoError(t, err, "executing seed SQL")
}
```

Call `SeedDB` inside `TestMain` after the container starts and migrations are applied.

---

## Step 6: Data Isolation Rules

All seed data used across tests must be fully isolated:

- **Per-test transactions:** Wrap each test body in `tx := db.Begin()` / `defer tx.Rollback()` so the DB state resets automatically after each test. Only call `tx.Commit()` when the test explicitly covers commit behavior.
- **Fresh IDs:** When a test inserts its own records, always call `uuid.New().String()` — never reuse IDs from the seed files, as ID collisions cause silent test interference.
- **Parallel safety:** If `t.Parallel()` is used, each parallel test must operate on a dedicated schema prefix or a uniquely-named dataset to prevent read/write races on shared rows.
- **Idempotent SQL:** Every `INSERT` in `seed.sql` must include `ON CONFLICT DO NOTHING` (PostgreSQL) or `INSERT OR IGNORE` (SQLite) so the seed script can be re-run without errors.

---

## Step 7: Directory Structure

Both the script and all generated files must be committed:

```
<package_dir>/
    testdata/
        gen/
            main.go          <- generator source (always committed)
            README.md        <- one-liner to regenerate
        users.json           <- faker-generated (committed)
        orders.json          <- faker-generated (committed)
        users.csv            <- faker-generated (committed)
        seed.sql             <- faker-generated (committed)
        edge_cases.json      <- hand-crafted (never regenerated)
```

The `testdata/gen/README.md` must contain:

```markdown
## Regenerate test data

Run this command from the package root whenever domain structs change or new scenarios are required:

    go run ./testdata/gen/main.go
```
