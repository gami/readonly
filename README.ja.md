# readonly

[English](README.md)

公開フィールドを維持したまま、外部パッケージからの再代入を静的解析で禁止する Go linter。

ORM・JSON シリアライズ・OpenAPI との互換性のためにフィールドを公開したいが、

```go
user.TenantID = "xxx"
user.Status = StatusDeleted
```

のような任意の書き換えは防ぎたい、というケースのためのツールです。

## 使い方

対象フィールドに `readonly:"external"` タグを付けます。

```go
type User struct {
    ID       string `readonly:"external"`
    TenantID string `readonly:"external"`
    Status   Status `readonly:"external"`

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
func (u *User) Activate() { u.Status = StatusActive }

// Struct Literal による初期化(コンストラクタ含む)
u := model.User{ID: id, TenantID: tenantID, Status: StatusActive}
```

禁止される操作(外部パッケージから):

```go
user.Status = StatusDeleted        // 直接代入
userPtr.Status = StatusDeleted     // ポインタ経由
order.User.Status = StatusDeleted  // ネストしたアクセス
users[i].Status = StatusDeleted    // スライス要素
counter.Value += 1                 // 複合代入
counter.Value++                    // インクリメント・デクリメント
admin.Status = StatusDeleted       // 埋め込みで昇格したフィールド
```

診断メッセージ:

```text
field User.Status is readonly outside package github.com/example/user
```

## 想定ユースケース

- **DDD Entity** — 状態変更をエンティティのメソッド経由に限定する
- **マルチテナント SaaS** — `TenantID` の誤った書き換えを防ぐ
- **監査上変更禁止の識別子** — 発番後の請求書番号などの変更を防ぐ

## 設計方針

`readonly:"external"` は完全な不変性(immutable)を意味しません。「**定義パッケージ外から見ると読み取り専用**」を意味します。定義パッケージ内では状態変更を許可し、外部からの直接変更のみを防止します。これにより:

- 公開フィールドを維持できる
- ORM や JSON シリアライズと両立できる
- 状態変更をドメインメソッドへ集約できる

## 非目標・既知の制限

- リフレクション・unsafe による変更の検出、実行時制御は対象外
- フィールドのアドレスを取ってポインタ越しに書き込むケース(`p := &u.Status; *p = x`)は検出しない

本 linter は静的解析による誤操作防止を目的とし、セキュリティ境界を提供するものではありません。
