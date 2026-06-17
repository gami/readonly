# readonly

[日本語](README.ja.md)

`readonly` is a Go linter that reports writes to struct fields tagged
`readonly:"..."` from outside the package that declares them.

The fields stay exported, so they keep working with ORM mapping, JSON
serialization, or generated OpenAPI types. What the linter blocks is code in
other packages doing:

```go
user.TenantID = "xxx"
user.Status = StatusDeleted
```

## Usage

Tag the fields you want to protect:

```go
type User struct {
    ID       string `readonly:"external"`
    TenantID string `readonly:"external"`
    Status   Status `readonly:"external"`

    Name string // untagged: freely assignable
}
```

There are two tag values:

- `readonly:"external"`: writable only inside the declaring package.
- `readonly:"immutable"`: never reassignable, anywhere. Set once via a
  composite literal.

```go
type Invoice struct {
    Number string `readonly:"immutable"`
}

inv := Invoice{Number: "INV-1"} // OK: initialization
inv.Number = "INV-2"            // reported, even inside the declaring package
```

Both values accept a `shallow` option, which protects only the field itself
and leaves its contents writable:

```go
type Cart struct {
    Lines []string `readonly:"external,shallow"`
}

cart.Lines = nil    // reported: reassignment of the field
cart.Lines[0] = "x" // allowed: contents stay writable
```

Run it directly:

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
including its tests. (The declaring package's own black-box tests, `package
user_test` alongside `user`, are always allowed.) So a repository or service
test that builds a fixture and then tweaks a protected field gets flagged.

The `-allow-all-test-files` flag exempts every `*_test.go` file, so test code
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

Forbidden from outside the declaring package:

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
field's contents by default. Opt out with the `shallow` option:

```go
type Account struct {
    Profile Profile  `readonly:"external"`
    Items   []string `readonly:"external"`
}

account.Profile.Name = "x" // forbidden: writes into a readonly field
account.Items[0] = "x"     // forbidden: element of a readonly field
```

An unrecognized tag value is reported at the declaration site, so a typo
cannot silently disable protection:

```go
Status Status `readonly:"externl"` // invalid readonly tag value "externl" (valid values: "external", "immutable")
```

The diagnostic looks like:

```text
field User.Status is readonly outside package github.com/example/user
```

## When this is useful

It is an app-level guard against accidental reassignment, not a replacement
for database constraints (foreign keys, row-level security), which stay the
primary defense for things like tenant isolation. Some cases where it helps:

- DDD entities, where state changes should go through the entity's own methods.
- Relation keys such as `Order.UserID`, which should not be silently
  re-pointed at a different parent after creation.
- Append-only records, where an event's `OccurredAt` and payload must stay
  fixed once recorded (`readonly:"immutable"`).
- Audit-protected identifiers like invoice numbers, fixed after issuance
  (`readonly:"immutable"`).

## `external` vs `immutable`

`readonly:"external"` is not full immutability. It means read-only as seen
from outside the declaring package: the owning package can still change the
field freely, but external direct writes are rejected. That keeps the field
exported (so ORM/JSON serialization keeps working) while concentrating state
transitions in the type's own methods.

Use `readonly:"immutable"` when you want no reassignment at all, including by
the owning package.

## Comparison with forbidigo

[forbidigo](https://github.com/ashanbrown/forbidigo) can also restrict access
to specific struct fields via `pkg.Type.Field` patterns (with `analyze-types`
enabled). The difference is where the rule lives:

- With readonly the owner of the type declares protection once, as a struct
  tag next to the field. With forbidigo every consuming repository lists the
  patterns in its own lint config and keeps them in sync as fields are added.
- readonly forbids writes only (reassignment, contents, whole-struct stores)
  and leaves reads alone. forbidigo matches identifier usage, so reads get
  flagged too unless the patterns are written carefully.
- readonly knows the write semantics, so same-package writes, composite
  literal initialization, and the declaring package's own `_test` package are
  allowed without any configuration.

forbidigo fits a consumer-side policy over identifiers in general; readonly
fits an invariant that belongs to the type itself.

## Limitations

- Writes via reflection or `unsafe`, and any runtime enforcement, are out of
  scope.
- Writes through the field's address are not detected, whether stored
  (`p := &u.Status; *p = x`) or passed to a function (`rows.Scan(&u.TenantID)`,
  `json.Unmarshal(data, &u.Status)`).

It is a static check for catching mistakes, not a security boundary.
