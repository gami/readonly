# readonly

[English](README.md)

公開フィールドを維持したまま、外部パッケージからの書き込みを静的解析で禁止する Go linter。

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

タグ値は2種類あります:

- `readonly:"external"` — 定義パッケージ内からのみ書き込み可能
- `readonly:"immutable"` — どこからも再代入不可。composite literal による生成時のみ値を設定できる

```go
type Invoice struct {
    Number string `readonly:"immutable"`
}

inv := Invoice{Number: "INV-1"} // OK: 生成時の値設定
inv.Number = "INV-2"            // 定義パッケージ内でも報告される
```

どちらのモードにも `shallow` オプションを付けられます。フィールド自体の再代入のみを禁止し、中身は書き込み可能のままにします:

```go
type Cart struct {
    Lines []string `readonly:"external,shallow"`
}

cart.Lines = nil    // 報告される: フィールド自体の再代入
cart.Lines[0] = "x" // 許可: 中身は書き込み可能
```

実行:

```sh
go run github.com/gami/readonly/cmd/readonly@latest ./...
```

または `go vet` 経由:

```sh
go install github.com/gami/readonly/cmd/readonly@latest
go vet -vettool=$(which readonly) ./...
```

## 判定ルール

許可される操作:

```go
// 同一パッケージ内からの代入
func (u *User) Activate() { u.Status = StatusActive }

// 定義パッケージ自身のブラックボックステスト(package user_test)
u.Status = StatusActive

// Struct Literal による初期化(コンストラクタ含む)
u := model.User{ID: id, TenantID: tenantID, Status: StatusActive}
```

禁止される操作(外部パッケージから):

```go
user.Status = StatusDeleted        // 直接代入
userPtr.Status = StatusDeleted     // ポインタ経由
order.User.Status = StatusDeleted  // ネストしたアクセス
users[i].Status = StatusDeleted    // スライス要素
user.TenantID += "-x"              // 複合代入(++ / -- も同様)
*userPtr = model.User{}            // ポインタ経由の構造体丸ごと代入
admin.Status = StatusDeleted       // 埋め込みで昇格したフィールド
```

構造体・スライス・マップ型のフィールドに付けた readonly タグは、デフォルトでフィールドの「中身」も保護します(`shallow` オプションで解除可能):

```go
type Account struct {
    Profile Profile  `readonly:"external"`
    Items   []string `readonly:"external"`
}

account.Profile.Name = "x" // 禁止: readonly フィールドの中身への書き込み
account.Items[0] = "x"     // 禁止: readonly フィールドの要素への書き込み
```

未知のタグ値は宣言時に報告されるため、typo で保護が無音のまま外れることはありません:

```go
Status Status `readonly:"externl"` // invalid readonly tag value "externl" (valid values: "external", "immutable")
```

診断メッセージ:

```text
field User.Status is readonly outside package github.com/example/user
```

## 想定ユースケース

- **DDD Entity** — 状態変更をエンティティのメソッド経由に限定する
- **マルチテナント SaaS** — `TenantID` の誤った書き換えを防ぐ
- **監査上変更禁止の識別子** — 発番後の請求書番号などの変更を防ぐ(`readonly:"immutable"`)

## 設計方針

`readonly:"external"` は完全な不変性(immutable)を意味しません。「**定義パッケージ外から見ると読み取り専用**」を意味します。定義パッケージ内では状態変更を許可し、外部からの直接変更のみを防止します。これにより:

- 公開フィールドを維持できる
- ORM や JSON シリアライズと両立できる
- 状態変更をドメインメソッドへ集約できる

定義パッケージ内からの再代入も含めて禁止したい場合は `readonly:"immutable"` を使います。

## 非目標・既知の制限

- リフレクション・unsafe による変更の検出、実行時制御は対象外
- フィールドのアドレス経由の書き込みは検出しない。ポインタを保存する形(`p := &u.Status; *p = x`)も、関数に渡す形(`rows.Scan(&u.TenantID)`、`json.Unmarshal(data, &u.Status)`)も同様

本 linter は静的解析による誤操作防止を目的とし、セキュリティ境界を提供するものではありません。
