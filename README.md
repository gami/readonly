# readonly

[日本語](README.ja.md)

A Go linter that forbids reassignment of struct fields from outside their declaring package, while keeping the fields exported.

Sometimes a field must stay exported for ORM mapping, JSON serialization, or OpenAPI compatibility, but you still want to prevent arbitrary writes like:

```go
user.TenantID = "xxx"
user.Status = StatusDeleted
```

This tool catches them with static analysis.

## Usage

Mark the fields you want to protect with the `readonly:"external"` tag.

```go
type User struct {
    ID       string `readonly:"external"`
    TenantID string `readonly:"external"`
    Status   Status `readonly:"external"`

    Name string // untagged: freely assignable
}
```

Run:

```sh
go run github.com/gami/readonly/cmd/readonly@latest ./...
```

Or via `go vet`:

```sh
go build -o readonly ./cmd/readonly
go vet -vettool=$(pwd)/readonly ./...
```

## Rules

Allowed:

```go
// Assignment within the declaring package
func (u *User) Activate() { u.Status = StatusActive }

// Initialization via composite literal (including constructors)
u := model.User{ID: id, TenantID: tenantID, Status: StatusActive}
```

Forbidden (from outside the declaring package):

```go
user.Status = StatusDeleted        // direct assignment
userPtr.Status = StatusDeleted     // through a pointer
order.User.Status = StatusDeleted  // nested access
users[i].Status = StatusDeleted    // slice element
counter.Value += 1                 // compound assignment
counter.Value++                    // increment / decrement
admin.Status = StatusDeleted       // field promoted via embedding
```

Diagnostic:

```text
field User.Status is readonly outside package github.com/example/user
```

## Use cases

- **DDD entities** — restrict state changes to the entity's own methods
- **Multi-tenant SaaS** — prevent accidental overwrites of `TenantID`
- **Audit-protected identifiers** — keep invoice numbers and the like fixed after issuance

## Design philosophy

`readonly:"external"` does **not** mean full immutability. It means *read-only as seen from outside the declaring package*: the owning package can freely change state, while external direct writes are rejected. This keeps fields exported and compatible with ORM/JSON serialization, while concentrating state transitions in domain methods.

## Non-goals and known limitations

- Writes via reflection or `unsafe`, and runtime enforcement, are out of scope
- Writes through a stored field address (`p := &u.Status; *p = x`) are not detected

This linter aims to prevent mistakes through static analysis, not to provide a security boundary.
