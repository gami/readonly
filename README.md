# readonly

[日本語](README.ja.md)

`readonly` is a Go linter that reports writes to struct fields tagged
`readonly:"..."`: from outside the declaring package (`external`), or from
anywhere (`immutable`).

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
    Lines  []string `readonly:"external,shallow"`
    Parent *Node    `readonly:"external,shallow"` // shared object owned elsewhere
}

cart.Lines = nil        // reported: reassignment of the field
cart.Lines[0] = "x"     // allowed: contents stay writable
cart.Parent.Name = "x"  // allowed: the pointee stays writable
```

To protect every field of a struct, put the tag on a blank field instead
of repeating it. A field's own tag takes precedence, and `readonly:"-"`
opts a field out:

```go
type Event struct {
    _ struct{} `readonly:"immutable"`

    ID         string           // immutable
    OccurredAt time.Time        // immutable
    Note       string `readonly:"-"` // writable
}
```

The blank field costs nothing at runtime and is ignored by `encoding/json`
and ORMs. It does make unkeyed literals (`Event{"id", t, ""}`) a compile
error in other packages, which is usually what you want for such types.

Run it directly:

```sh
go run github.com/gami/readonly/cmd/readonly@latest ./...
```

Or via `go vet`:

```sh
go install github.com/gami/readonly/cmd/readonly@latest
go vet -vettool=$(which readonly) ./...
```

To embed it in your own checker, use
`readonly.NewAnalyzer(readonly.Options{...})`.

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
including its tests. (For `external`, the declaring package's own black-box
tests, `package user_test`, are allowed.) So a repository or service test
that builds a fixture and then tweaks a protected field gets flagged.

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

### Reporting address-of

By default only assignments and the `delete`, `clear`, and `copy` builtins
count as writes. Passing the address of a readonly field is not reported,
even though that is how ORM and JSON code writes:

```go
rows.Scan(&u.ID)
json.Unmarshal(data, &u.Status)
account.Profile.SetName("x") // pointer receiver: implicitly &account.Profile
```

The `-report-address-of` flag reports these:

```sh
readonly -report-address-of ./...
```

```yaml
    custom:
      readonly:
        type: module
        settings:
          report-address-of: true
```

With the flag on, taking the address of a readonly field (or of its
contents, unless `shallow`) is reported in these cases:

```go
rows.Scan(&u.ID)                // passed to a call
account.Profile.SetName("x")    // used as a pointer receiver
f := account.Profile.SetName    // same, as a method value

p := &u.ID                      // saved in a variable that is later
*p = "x"                        //   written through,
rows.Scan(p)                    //   passed to a call,
q := &account.Profile
q.SetName("x")                  //   or used as a pointer receiver
```

Not reported:

```go
resp.ID = &u.ID                 // storing the pointer is not a write
api.User{ID: &u.ID}             // same
json.Unmarshal(data, &u)        // the whole struct, not a field
p := &u.ID; _ = *p              // p is only read
u.Activate()                    // pointer receiver on the type itself
```

The flag is off by default because the linter cannot tell a call that
writes from one that reads: `fmt.Println(&u.ID)` is reported too. If you
pass `&u.Field` to helpers for reading, use a helper that takes a value
instead, such as `ptr(u.ID)` with `func ptr[T any](v T) *T`.

`immutable` forbids writes everywhere, so `rows.Scan(&inv.Number)` is
reported inside the declaring package too.

## Rules

Allowed:

```go
// Assignment within the declaring package
func (u *User) Activate() { u.Status = StatusActive }

// The declaring package's own black-box tests (package user_test)
u.Status = StatusActive

// Initialization via composite literal (including constructors)
u := model.User{ID: id, TenantID: tenantID, Status: StatusActive}

// Reassigning a variable of the current package: treated like initialization
u = model.User{ID: id}

// Swapping a reference: the referenced contents are untouched
team.Owner = &other   // *User field
team.Guests = nil     // []User field

// Setting a map entry: replaces which value the key holds
byID[u.ID] = u
```

Forbidden from outside the declaring package:

```go
user.Status = StatusDeleted        // direct assignment
userPtr.Status = StatusDeleted     // through a pointer
order.User.Status = StatusDeleted  // nested access
users[i].Status = StatusDeleted    // slice element
user.TenantID += "-x"              // compound assignment, ++ and -- too
admin.Status = StatusDeleted       // field promoted via embedding
```

Assigning a whole struct to a place that already holds one replaces every
field at once, readonly ones included, so it is reported. This also applies
when the readonly field sits inside a nested or embedded struct or an array
element. Assigning to a variable of the current package or to a map entry is
not reported (see above); a variable of another package (`model.Default =
model.User{}`) is that package's storage and is reported.

```go
*userPtr = model.User{}      // through a pointer
users[i] = model.User{}      // slice element
order.User = model.User{}    // untagged field holding a User
*admin = Admin{}             // Admin embeds model.User
```

A readonly tag on a struct-, slice-, map-, or pointer-typed field also
protects the field's contents by default: sub-fields, elements, and the
pointee, including through the `delete`, `clear`, and `copy` builtins. Opt
out with the `shallow` option:

```go
type Account struct {
    Profile Profile           `readonly:"external"`
    Items   []string          `readonly:"external"`
    Meta    map[string]string `readonly:"external"`
    Ref     *Profile          `readonly:"external"`
}

account.Profile.Name = "x"  // forbidden: writes into a readonly field
account.Items[0] = "x"      // forbidden: element of a readonly field
copy(account.Items, src)    // forbidden: overwrites elements
delete(account.Meta, "k")   // forbidden: removes an entry
clear(account.Meta)         // forbidden: removes every entry
account.Ref.Name = "x"      // forbidden: writes through a readonly pointer
*account.Ref = Profile{}    // forbidden: overwrites the pointee
```

An unrecognized tag value, and a tag key that differs from `readonly` only
in case, are reported at the declaration site, so a typo cannot silently
disable protection:

```go
Status Status `readonly:"externl"` // invalid readonly tag value "externl" (valid values: "external", "immutable", "-")
Status Status `ReadOnly:"external"` // unrecognized struct tag key "ReadOnly" (did you mean "readonly"?)
```

The diagnostic looks like:

```text
field User.Status is readonly outside package github.com/example/user
```

With `-report-address-of`, a write through a saved pointer also names the
pointer and where its address was taken:

```text
field User.ID is readonly outside package github.com/example/user (written through p, address taken at repo.go:42:7)
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
- Writes through an alias of a slice, map, or pointer field are not detected:
  `items := a.Items; items[0] = "x"`, `sort.Strings(a.Items)`,
  `mutate(a.Ref)`, `append(a.Items[:0], "x")`. Only `delete`, `clear`, and
  `copy` are recognized as functions that write into their argument.
- Writes through the field's address (`rows.Scan(&u.TenantID)`,
  `account.Profile.SetName("x")`, `p := &u.Status; *p = x`) are only detected
  with `-report-address-of`, and even then only as described above. A
  pointer stored in an interface (`var s Setter = &a.Profile;
  s.SetName("x")`) is not detected even with the flag.
- With `-report-address-of`, a pointer is tracked by variable, not by flow.
  A copy (`q := p`) and a pointer returned, stored in a struct, or received
  from another function are not tracked. Rebinding the variable (`p = other`)
  does not stop tracking it, so a later `*p = x` is still reported. Writes
  through a tracked pointer are reported even when they only touch contents
  that `shallow` would allow.
- Passing the address of a whole struct (`json.Unmarshal(data, &u)`,
  `db.Find(&users[0])`) is never reported, so that loading a struct stays
  possible.
- Writes to a copy are reported just like writes to the original. A value
  parameter or a `for _, u := range users` variable of a readonly-bearing type
  is still flagged when its field is assigned.

It is a static check for catching mistakes, not a security boundary.
