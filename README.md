# readonly

[日本語](README.ja.md)

A Go linter that forbids reassignment of struct fields from outside their declaring package, while keeping the fields exported.

Sometimes a field must stay exported for ORM mapping or JSON serialization, but you still want to prevent arbitrary writes like:

```go
user.TenantID = "xxx"
user.Status = StatusDeleted
```

This tool catches them with static analysis.

## Usage

Mark the fields you want to protect with the `reassign:"internal"` tag.

```go
type User struct {
    ID       string `reassign:"internal"`
    TenantID string `reassign:"internal"`
    Status   Status `reassign:"internal"`

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
func (u *User) ChangeStatus(s Status) { u.Status = s }

// Initialization via composite literal
u := model.User{ID: id, TenantID: tenantID, Status: StatusActive}
```

Forbidden (from outside the declaring package):

```go
user.Status = StatusDeleted        // direct assignment
userPtr.Status = StatusDeleted     // through a pointer
order.User.Status = StatusDeleted  // nested access
users[i].Status = StatusDeleted    // slice element
user.TenantID += "-x"              // compound assignment
admin.Status = StatusDeleted       // field promoted via embedding
```

Diagnostic:

```text
field User.Status is marked reassign:"internal" and cannot be modified outside package github.com/example/user
```

## Use cases

- **DDD entities** — restrict state changes to the entity's own methods
- **Multi-tenancy** — prevent accidental overwrites of `TenantID`
- **Audit-protected identifiers** — keep invoice numbers and the like immutable after issuance

## Non-goals and known limitations

- Writes via reflection or `unsafe`, and runtime enforcement, are out of scope
- Writes through a stored field address (`p := &u.Status; *p = x`) are not detected

This linter aims to prevent mistakes through static analysis, not to provide a security boundary.

## Planned extensions

- `reassign:"immutable"` — no reassignment anywhere (composite literals only)
- `reassign:"package"` — alias of `internal`
- `reassign:"friend=github.com/example/service"` — allow reassignment only from specific packages
