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
    Lines  []string `readonly:"external,shallow"`
    Parent *Node    `readonly:"external,shallow"` // 他所が所有する共有オブジェクト
}

cart.Lines = nil        // 報告される: フィールド自体の再代入
cart.Lines[0] = "x"     // 許可: 中身は書き込み可能
cart.Parent.Name = "x"  // 許可: 参照先は書き込み可能
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

// 裸の変数への再代入: 初期化と同じ扱い
u = model.User{ID: id}

// 参照の差し替え: 参照先の中身は書き換わらない
team.Owner = &other   // *User 型のフィールド
team.Guests = nil     // []User 型のフィールド
```

外部パッケージから禁止される操作:

```go
user.Status = StatusDeleted        // 直接代入
userPtr.Status = StatusDeleted     // ポインタ経由
order.User.Status = StatusDeleted  // ネストしたアクセス
users[i].Status = StatusDeleted    // スライス要素
user.TenantID += "-x"              // 複合代入(++ / -- も同様)
admin.Status = StatusDeleted       // 埋め込みで昇格したフィールド
```

既存の格納先に構造体を丸ごと代入すると中のフィールドがすべて上書きされるため、
その値が readonly フィールドを(直接、または入れ子の構造体・埋め込み・配列として
値で)含んでいれば禁止されます:

```go
*userPtr = model.User{}      // ポインタ経由
users[i] = model.User{}      // スライス要素
byID["x"] = model.User{}     // マップ要素
order.User = model.User{}    // User を保持するタグなしフィールド
*admin = Admin{}             // Admin は model.User を埋め込んでいる
```

構造体・スライス・マップ・ポインタ型のフィールドに付けた readonly タグは、
デフォルトでフィールドの中身(サブフィールド、要素、参照先)も保護します。
組み込み関数の `delete` / `clear` / `copy` による書き換えも対象です。`shallow`
オプションで解除できます。

```go
type Account struct {
    Profile Profile           `readonly:"external"`
    Items   []string          `readonly:"external"`
    Meta    map[string]string `readonly:"external"`
    Ref     *Profile          `readonly:"external"`
}

account.Profile.Name = "x"  // 禁止: readonly フィールドの中身への書き込み
account.Items[0] = "x"      // 禁止: readonly フィールドの要素への書き込み
copy(account.Items, src)    // 禁止: 要素の上書き
delete(account.Meta, "k")   // 禁止: エントリの削除
clear(account.Meta)         // 禁止: 全エントリの削除
account.Ref.Name = "x"      // 禁止: readonly ポインタ経由の書き込み
*account.Ref = Profile{}    // 禁止: 参照先の上書き
```

上の丸ごと代入との非対称に注意してください。タグ付きフィールドの「中身」は
ポインタの先まで含みます。`account.Ref.Name = "x"` は実際に参照先を書き換える
からです。一方、丸ごと代入が上書きするのは構造体が値で保持しているものだけなので、
`*order = Order{}` は `order.UserPtr` が指していた先への書き込みではありません。
どちらも「その代入で実際に書き換わるメモリはどこか」で決まっています。

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
- ポインタレシーバのメソッド経由で readonly フィールドの中身を変更する呼び出し
  (`account.Profile.SetName("x")`)は検出しません。書き込みとして扱うのは代入と
  組み込み関数の `delete` / `clear` / `copy` だけです。
- コピーへの書き込みも元の値への書き込みと同じように報告されます。値渡しの引数や
  `for _, u := range users` の変数でも、readonly フィールドに代入すれば検出されます。

これは誤操作を静的解析で捕まえるためのものであり、セキュリティ境界ではありません。
