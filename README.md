# readonly

[日本語](README.ja.md)

A Go linter that forbids writes to struct fields from outside their declaring package, while keeping the fields exported.

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

Two tag values are supported:

- `readonly:"external"` — writable only inside the declaring package
- `readonly:"immutable"` — never reassignable, anywhere; the value is set
  once via a composite literal

```go
type Invoice struct {
    Number string `readonly:"immutable"`
}

inv := Invoice{Number: "INV-1"} // OK: initialization
inv.Number = "INV-2"            // reported, even inside the declaring package
```

Both modes accept the `shallow` option, which protects only the field
itself and leaves its contents writable:

```go
type Cart struct {
    Lines []string `readonly:"external,shallow"`
}

cart.Lines = nil    // reported: reassignment of the field
cart.Lines[0] = "x" // allowed: contents stay writable
```

Run:

```sh
go run github.com/gami/readonly/cmd/readonly@latest ./...
```

Or via `go vet`:

```sh
go install github.com/gami/readonly/cmd/readonly@latest
go vet -vettool=$(which readonly) ./...
```

### golangci-lint

readonly ships as a [module plugin](https://golangci-lint.run/plugins/module-plugins/). Put `.custom-gcl.yml` in your repository:

```yaml
version: v2.9.0 # your golangci-lint version
plugins:
  - module: 'github.com/gami/readonly'
    import: 'github.com/gami/readonly/plugin'
    version: latest # or pin a specific version
```

Build the custom binary once (and after version bumps):

```sh
golangci-lint custom
```

Enable the linter in `.golangci.yml`:

```yaml
version: "2"
linters:
  enable:
    - readonly
  settings:
    custom:
      readonly:
        type: module
        description: Forbids writes to readonly-tagged struct fields.
```

Then run `./custom-gcl run ./...`. Suppression via `//nolint:readonly` works as usual.

### Allowing writes in test files

By default, writes are reported in any package other than the declaring one,
including its tests. (The declaring package's *own* tests — `package
user_test` alongside `user` — are always allowed.) Repository or service tests
that build a fixture and then tweak a protected field therefore get flagged.

Enable `-allow-all-test-files` to exempt every `*_test.go` file, so test code
anywhere can mutate readonly fields while production code stays protected:

```sh
readonly -allow-all-test-files ./...
```

With golangci-lint, set it under the linter's `settings`:

```yaml
    custom:
      readonly:
        type: module
        settings:
          allow-all-test-files: true
```

## Rules

Allowed:

```go
// Assignment within the declaring package
func (u *User) Activate() { u.Status = StatusActive }

// The declaring package's own black-box tests (package user_test)
u.Status = StatusActive

// Initialization via composite literal (including constructors)
u := model.User{ID: id, TenantID: tenantID, Status: StatusActive}
```

Forbidden (from outside the declaring package):

```go
user.Status = StatusDeleted        // direct assignment
userPtr.Status = StatusDeleted     // through a pointer
order.User.Status = StatusDeleted  // nested access
users[i].Status = StatusDeleted    // slice element
user.TenantID += "-x"              // compound assignment, ++ and -- too
*userPtr = model.User{}            // whole-struct store through a pointer
admin.Status = StatusDeleted       // field promoted via embedding
```

A readonly tag on a struct-, slice-, or map-typed field also protects the
field's *contents* by default (opt out with the `shallow` option):

```go
type Account struct {
    Profile Profile  `readonly:"external"`
    Items   []string `readonly:"external"`
}

account.Profile.Name = "x" // forbidden: writes into a readonly field
account.Items[0] = "x"     // forbidden: element of a readonly field
```

Unrecognized tag values are reported at the declaration site, so a typo
cannot silently disable protection:

```go
Status Status `readonly:"externl"` // invalid readonly tag value "externl" (valid values: "external", "immutable")
```

Diagnostic:

```text
field User.Status is readonly outside package github.com/example/user
```

## Use cases

- **DDD entities** — restrict state changes to the entity's own methods
- **Multi-tenant SaaS** — prevent accidental overwrites of `TenantID`
- **Audit-protected identifiers** — keep invoice numbers and the like fixed after issuance (`readonly:"immutable"`)

## Design philosophy

`readonly:"external"` does **not** mean full immutability. It means *read-only as seen from outside the declaring package*: the owning package can freely change state, while external direct writes are rejected. This keeps fields exported and compatible with ORM/JSON serialization, while concentrating state transitions in domain methods.

When you do want full immutability — no reassignment even by the owning package — use `readonly:"immutable"`.

## Comparison with forbidigo

[forbidigo](https://github.com/ashanbrown/forbidigo) can also restrict access to specific struct fields via `pkg.Type.Field` patterns (with `analyze-types` enabled), but the two tools place the responsibility differently:

- **Who declares the rule.** With readonly, the *owner of the type* declares protection once, as a struct tag next to the field. With forbidigo, every *consuming repository* must list the right patterns in its lint configuration — and keep them in sync as fields are added.
- **What is forbidden.** readonly forbids *writes* only (reassignment, contents, whole-struct stores) and leaves reads untouched. forbidigo matches identifier usage, so reads are flagged too unless patterns are carefully engineered.
- **Built-in allowances.** readonly understands the write semantics: same-package writes, composite literal initialization, and the declaring package's own `_test` package are allowed without any configuration.

Use forbidigo when you want a consumer-side policy over identifiers in general; use readonly when the invariant belongs to the type itself.

## Non-goals and known limitations

- Writes via reflection or `unsafe`, and runtime enforcement, are out of scope
- Writes through the field's address are not detected, whether stored
  (`p := &u.Status; *p = x`) or passed to a function
  (`rows.Scan(&u.TenantID)`, `json.Unmarshal(data, &u.Status)`)

This linter aims to prevent mistakes through static analysis, not to provide a security boundary.
