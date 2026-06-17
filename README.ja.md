# readonly

[English](README.md)

`readonly` は、`readonly:"..."` タグを付けた構造体フィールドへの、定義パッケージ
外からの書き込みを報告する Go linter です。

フィールドは公開のままなので、ORM・JSON シリアライズ・生成された OpenAPI 型と
そのまま使えます。この linter が止めるのは、別パッケージのコードによる次のような
書き換えです:

```go
user.TenantID = "xxx"
user.Status = StatusDeleted
```

## 使い方

保護したいフィールドにタグを付けます。

```go
type User struct {
    ID       string `readonly:"external"`
    TenantID string `readonly:"external"`
    Status   Status `readonly:"external"`

    Name string // タグなし: 自由に代入可能
}
```

タグ値は2種類です。

- `readonly:"external"`: 定義パッケージ内からのみ書き込み可能。
- `readonly:"immutable"`: どこからも再代入不可。composite literal による生成時に
  一度だけ値を設定できる。

```go
type Invoice struct {
    Number string `readonly:"immutable"`
}

inv := Invoice{Number: "INV-1"} // OK: 生成時の値設定
inv.Number = "INV-2"            // 定義パッケージ内でも報告される
```

どちらの値にも `shallow` オプションを付けられます。フィールド自体の再代入だけを
禁止し、中身は書き込み可能のままにします。

```go
type Cart struct {
    Lines []string `readonly:"external,shallow"`
}

cart.Lines = nil    // 報告される: フィールド自体の再代入
cart.Lines[0] = "x" // 許可: 中身は書き込み可能
```

直接実行する場合:

```sh
go run github.com/gami/readonly/cmd/readonly@latest ./...
```

`go vet` 経由の場合:

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

デフォルトでは、定義パッケージ以外のすべてのパッケージで書き込みが報告されます
(テストを含む)。ただし定義パッケージ自身のブラックボックステスト(`user` と同じ
場所の `package user_test`)は常に許可されます。そのため、repository やサービスの
テストで fixture を作って保護フィールドを差し替える操作は検出されてしまいます。

`-allow-all-test-files` を有効にすると、すべての `*_test.go` ファイルが対象外に
なり、テストコードはどこからでも readonly フィールドを変更できます(本番コードは
保護されたまま):

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

// composite literal による初期化(コンストラクタ含む)
u := model.User{ID: id, TenantID: tenantID, Status: StatusActive}
```

外部パッケージから禁止される操作:

```go
user.Status = StatusDeleted        // 直接代入
userPtr.Status = StatusDeleted     // ポインタ経由
order.User.Status = StatusDeleted  // ネストしたアクセス
users[i].Status = StatusDeleted    // スライス要素
user.TenantID += "-x"              // 複合代入(++ / -- も同様)
*userPtr = model.User{}            // ポインタ経由の構造体丸ごと代入
admin.Status = StatusDeleted       // 埋め込みで昇格したフィールド
```

構造体・スライス・マップ型のフィールドに付けた readonly タグは、デフォルトで
フィールドの中身も保護します。`shallow` オプションで解除できます。

```go
type Account struct {
    Profile Profile  `readonly:"external"`
    Items   []string `readonly:"external"`
}

account.Profile.Name = "x" // 禁止: readonly フィールドの中身への書き込み
account.Items[0] = "x"     // 禁止: readonly フィールドの要素への書き込み
```

未知のタグ値は宣言時に報告されるため、typo で保護が無音のまま外れることは
ありません:

```go
Status Status `readonly:"externl"` // invalid readonly tag value "externl" (valid values: "external", "immutable")
```

診断メッセージは次のようになります:

```text
field User.Status is readonly outside package github.com/example/user
```

## どんなときに役立つか

これは DB レベルの制約(外部キー、RLS)を置き換えるものではなく、それらを補完する
アプリケーション層での誤代入ガードです。テナント分離のような本質的な防御は引き続き
DB 側で行うのが基本です。役立つ場面の例:

- DDD のエンティティで、状態変更をエンティティ自身のメソッド経由に限定したいとき。
- `Order.UserID` のようなリレーションキーが、生成後に別の親へ勝手に付け替えられる
  のを防ぎたいとき。
- 追記専用レコードで、記録済みイベントの `OccurredAt` やペイロードを固定したいとき
  (`readonly:"immutable"`)。
- 請求書番号のような監査上の識別子を、発番後に固定したいとき
  (`readonly:"immutable"`)。

## `external` と `immutable` の違い

`readonly:"external"` は完全な不変性ではありません。「定義パッケージの外から見ると
読み取り専用」という意味です。所有パッケージ内では自由に状態を変更でき、外部からの
直接書き込みだけを拒否します。これにより、フィールドを公開したまま(ORM や JSON
シリアライズと両立したまま)、状態変更を型自身のメソッドへ集約できます。

所有パッケージからの再代入も含めて一切禁止したい場合は `readonly:"immutable"` を
使います。

## forbidigo との違い

[forbidigo](https://github.com/ashanbrown/forbidigo) も `analyze-types` を有効に
すれば `pkg.Type.Field` パターンで特定フィールドへのアクセスを禁止できます。違いは
ルールの置き場所です。

- readonly は型の所有者が、フィールドの隣の struct タグとして一度だけ宣言します。
  forbidigo は利用側の各リポジトリが lint 設定にパターンを列挙し、フィールド追加の
  たびに同期し続ける必要があります。
- readonly は書き込みのみ(再代入・中身・丸ごと代入)を禁止し、読み取りには触れません。
  forbidigo は識別子の使用にマッチするため、パターンを工夫しない限り読み取りも
  検出されます。
- readonly は書き込みのセマンティクスを理解しているので、同一パッケージ内の書き込み・
  composite literal による初期化・定義パッケージ自身の `_test` パッケージを、設定なし
  で許可します。

識別子全般に対する利用側ポリシーが欲しいなら forbidigo、不変条件が型そのものに
属するなら readonly が向いています。

## 制限

- リフレクションや unsafe による変更の検出、実行時の制御は対象外です。
- フィールドのアドレス経由の書き込みは検出しません。ポインタを保存する形
  (`p := &u.Status; *p = x`)も、関数に渡す形(`rows.Scan(&u.TenantID)`、
  `json.Unmarshal(data, &u.Status)`)も同様です。

これは誤操作を静的解析で捕まえるためのものであり、セキュリティ境界ではありません。
