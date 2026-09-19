# readonly

[English](README.md)

`readonly` は、`readonly:"..."` タグを付けた構造体フィールドへの書き込みを報告する
Go linter です。`external` なら定義パッケージ外からの書き込みを、`immutable` なら
どこからの書き込みも報告します。

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

自前の checker に組み込むには `readonly.NewAnalyzer(readonly.Options{...})` を
使います。

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
(テストを含む)。`external` の場合、定義パッケージ自身のブラックボックステスト
(`package user_test`)は許可されます。そのため、repository やサービスのテストで
fixture を作って保護フィールドを差し替える操作は検出されてしまいます。

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

### アドレス取得を報告する

デフォルトで書き込みとみなすのは、代入と組み込み関数の `delete` / `clear` /
`copy` だけです。readonly フィールドのアドレスを渡す形は報告されません。ORM や
JSON のコードはこの形で書き込みます:

```go
rows.Scan(&u.ID)
json.Unmarshal(data, &u.Status)
account.Profile.SetName("x") // ポインタレシーバ: 暗黙的に &account.Profile
```

`-report-address-of` を有効にすると、これらが報告されます:

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

有効時は、readonly フィールド(`shallow` でなければその中身も)のアドレスを取る
次の形が報告されます:

```go
rows.Scan(&u.ID)                // 関数に渡す
account.Profile.SetName("x")    // ポインタレシーバとして使う

p := &u.ID                      // 変数に保存し、その変数を後で
*p = "x"                        //   書き込みに使う、
rows.Scan(p)                    //   関数に渡す、
q := &account.Profile
q.SetName("x")                  //   ポインタレシーバとして使う
```

報告されないもの:

```go
resp.ID = &u.ID                 // ポインタを格納しても書き込みは起きない
api.User{ID: &u.ID}             // 同上
json.Unmarshal(data, &u)        // フィールドではなく構造体全体
p := &u.ID; _ = *p              // p を読むだけ
u.Activate()                    // 型自身のポインタレシーバ
```

デフォルトで無効なのは、書き込む呼び出しと読むだけの呼び出しを区別できない
からです。`fmt.Println(&u.ID)` も報告されます。読み取り目的で `&u.Field` を
ヘルパーに渡している場合は、`ptr(u.ID)`(`func ptr[T any](v T) *T`)のように
値を受け取るヘルパーに置き換えてください。

`immutable` はどこからの書き込みも禁止するため、定義パッケージ内の
`rows.Scan(&inv.Number)` も報告されます。

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

既に構造体が入っている場所に構造体を丸ごと代入すると、readonly フィールドも
含めて全フィールドが置き換わるため報告されます。readonly フィールドが入れ子や
埋め込みの構造体、配列要素の中にある場合も同様です。裸の変数への代入は報告
されません(上記参照)。

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

未知のタグ値は宣言時に報告されるため、typo で保護が無音のまま外れることは
ありません:

```go
Status Status `readonly:"externl"` // invalid readonly tag value "externl" (valid values: "external", "immutable")
```

診断メッセージは次のようになります:

```text
field User.Status is readonly outside package github.com/example/user
```

`-report-address-of` で保存したポインタ経由の書き込みを報告するときは、
ポインタ変数名とアドレスを取った位置も含まれます:

```text
field User.ID is readonly outside package github.com/example/user (written through p, address taken at repo.go:42:7)
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
- フィールドのアドレス経由の書き込み(`rows.Scan(&u.TenantID)`、
  `account.Profile.SetName("x")`、`p := &u.Status; *p = x`)は
  `-report-address-of` を有効にしたときだけ、上述の範囲で検出されます。
- `-report-address-of` のポインタ追跡は変数単位で、フローは追いません。コピー
  (`q := p`)、返したポインタ、構造体に格納したポインタ、他の関数から受け取った
  ポインタは追跡しません。変数を別のポインタに再代入(`p = other`)しても追跡は
  続くので、その後の `*p = x` は報告されます。追跡中のポインタ経由の書き込みは、
  `shallow` なら許可されるはずの中身への書き込みでも報告されます。
- 構造体全体のアドレスを渡す形(`json.Unmarshal(data, &u)`、`db.Find(&users[0])`)
  は報告しません。構造体のロードができなくなるためです。
- コピーへの書き込みも元の値への書き込みと同じように報告されます。値渡しの引数や
  `for _, u := range users` の変数でも、readonly フィールドに代入すれば検出されます。

これは誤操作を静的解析で捕まえるためのものであり、セキュリティ境界ではありません。
