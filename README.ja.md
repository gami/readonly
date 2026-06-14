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

### golangci-lint

[module plugin](https://golangci-lint.run/plugins/module-plugins/) として golangci-lint に組み込めます。リポジトリに `.custom-gcl.yml` を置きます:

```yaml
version: v2.9.0 # 使用している golangci-lint のバージョン
plugins:
  - module: 'github.com/gami/readonly'
    import: 'github.com/gami/readonly/plugin'
    version: latest # 特定バージョンへの固定も可
```

カスタムバイナリをビルドします(初回とバージョン更新時のみ):

```sh
golangci-lint custom
```

`.golangci.yml` で有効化します:

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

あとは `./custom-gcl run ./...` で実行できます。`//nolint:readonly` による抑制も通常通り機能します。

### テストファイルでの書き込みを許可する

デフォルトでは、定義パッケージ以外のすべてのパッケージで書き込みが報告されます(テストを含む)。ただし定義パッケージ**自身**のテスト(`user` と同じ場所の `package user_test`)は常に許可されます。そのため、repository やサービスのテストで fixture を作って protected フィールドを差し替える操作は検出されてしまいます。

`-allow-all-test-files` を有効にすると、すべての `*_test.go` ファイルが対象外になり、テストコードはどこからでも readonly フィールドを変更できます(本番コードは保護されたまま):

```sh
readonly -allow-all-test-files ./...
```

golangci-lint では linter の `settings` に指定します:

```yaml
    custom:
      readonly:
        type: module
        settings:
          allow-all-test-files: true
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

## forbidigo との違い

[forbidigo](https://github.com/ashanbrown/forbidigo) も `analyze-types` を有効にすれば `pkg.Type.Field` パターンで特定フィールドへのアクセスを禁止できますが、責務の置き場所が異なります:

- **誰がルールを宣言するか。** readonly は**型の所有者**が struct タグとしてフィールドの隣に一度だけ宣言します。forbidigo は**利用側の各リポジトリ**が lint 設定にパターンを列挙し、フィールド追加のたびに同期し続ける必要があります
- **何を禁止するか。** readonly は**書き込みのみ**を禁止し(再代入・中身・丸ごと代入)、読み取りは自由です。forbidigo は識別子の使用にマッチするため、パターンを工夫しない限り読み取りも検出されます
- **組み込みの許可ルール。** readonly は書き込みのセマンティクスを理解しており、同一パッケージ・composite literal による初期化・定義パッケージ自身の `_test` パッケージを設定なしで許可します

識別子全般に対する利用側ポリシーが欲しいなら forbidigo、不変条件が型そのものに属するなら readonly が向いています。

## 非目標・既知の制限

- リフレクション・unsafe による変更の検出、実行時制御は対象外
- フィールドのアドレス経由の書き込みは検出しない。ポインタを保存する形(`p := &u.Status; *p = x`)も、関数に渡す形(`rows.Scan(&u.TenantID)`、`json.Unmarshal(data, &u.Status)`)も同様

本 linter は静的解析による誤操作防止を目的とし、セキュリティ境界を提供するものではありません。
