# readonly

公開フィールドを維持したまま、外部パッケージからの再代入を静的解析で禁止する Go linter。

ORM や JSON シリアライズの都合でフィールドを公開したいが、

```go
user.TenantID = "xxx"
user.Status = StatusDeleted
```

のような任意の書き換えは防ぎたい、というケースのためのツールです。

## 使い方

対象フィールドに `reassign:"internal"` タグを付けます。

```go
type User struct {
    ID       string `reassign:"internal"`
    TenantID string `reassign:"internal"`
    Status   Status `reassign:"internal"`

    Name string // タグなし: 自由に代入可能
}
```

実行:

```sh
go run github.com/gami/readonly/cmd/readonly@latest ./...
```

または `go vet` 経由:

```sh
go build -o readonly ./cmd/readonly
go vet -vettool=$(pwd)/readonly ./...
```

## 判定ルール

許可される操作:

```go
// 同一パッケージ内からの代入
func (u *User) ChangeStatus(s Status) { u.Status = s }

// Struct Literal による初期化(生成時の値設定)
u := model.User{ID: id, TenantID: tenantID, Status: StatusActive}
```

禁止される操作(外部パッケージから):

```go
user.Status = StatusDeleted        // 直接代入
userPtr.Status = StatusDeleted     // ポインタ経由
order.User.Status = StatusDeleted  // ネストしたアクセス
users[i].Status = StatusDeleted    // スライス要素
user.TenantID += "-x"              // 複合代入
admin.Status = StatusDeleted       // 埋め込みで昇格したフィールド
```

診断メッセージ:

```text
field User.Status is marked reassign:"internal" and cannot be modified outside package github.com/example/user
```

## 想定ユースケース

- **DDD Entity** — 状態変更をエンティティのメソッド経由に限定する
- **マルチテナント** — `TenantID` の誤った書き換えを防ぐ
- **監査上変更禁止の識別子** — 発番後の請求書番号などの変更を防ぐ

## 非目標・既知の制限

- リフレクション・unsafe による変更の検出、実行時制御は対象外
- フィールドのアドレスを取ってポインタ越しに書き込むケース(`p := &u.Status; *p = x`)は検出しない

本 linter は静的解析による誤操作防止を目的とします。

## 今後の拡張案

- `reassign:"immutable"` — どこからも再代入不可(struct literal のみ許可)
- `reassign:"package"` — `internal` の別名
- `reassign:"friend=github.com/example/service"` — 特定パッケージからのみ許可
