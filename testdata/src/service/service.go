package service

import (
	"fmt"

	"model"
)

// 外部パッケージからの代入は禁止される。
func direct(user model.User) {
	user.Status = model.StatusDeleted // want `field User\.Status is readonly outside package model`
}

// ポインタ経由も禁止される。
func viaPointer(userPtr *model.User) {
	userPtr.Status = model.StatusDeleted // want `field User\.Status is readonly outside package model`
}

// ネストしたアクセスも禁止される。
func nested(order *model.Order) {
	order.User.Status = model.StatusDeleted // want `field User\.Status is readonly outside package model`
}

// スライス要素も禁止される。
func sliceElem(users []model.User, i int) {
	users[i].Status = model.StatusDeleted // want `field User\.Status is readonly outside package model`
}

// 多重代入の各左辺もそれぞれ検査される。
func multiAssign(user *model.User) {
	user.ID, user.TenantID = "a", "b" // want `field User\.ID is readonly outside package model` `field User\.TenantID is readonly outside package model`
}

// 複合代入も禁止される。
func compound(user *model.User) {
	user.TenantID += "-suffix" // want `field User\.TenantID is readonly outside package model`
}

// インクリメント・デクリメントも禁止される。
func incDec(counter *model.Counter) {
	counter.Value++ // want `field Counter\.Value is readonly outside package model`
	counter.Value-- // want `field Counter\.Value is readonly outside package model`
}

// 埋め込みで昇格したフィールドへの代入も禁止される。
type Admin struct {
	model.User
}

func promoted(admin *Admin) {
	admin.Status = model.StatusDeleted // want `field User\.Status is readonly outside package model`
}

// タグ付きフィールドの「中身」への書き込みも禁止される。
func interior(account *model.Account) {
	account.Profile.Name = "x" // want `field Account\.Profile is readonly outside package model`
	account.Items[0] = "x"     // want `field Account\.Items is readonly outside package model`
	account.Meta["k"] = "v"    // want `field Account\.Meta is readonly outside package model`
}

// ポインタ型のタグ付きフィールドは、参照先が「中身」として保護される。
func pointerInterior(account *model.Account, p *model.Profile) {
	account.Ref.Name = "x"         // want `field Account\.Ref is readonly outside package model`
	*account.Ref = model.Profile{} // want `field Account\.Ref is readonly outside package model`
	account.Ref = p                // want `field Account\.Ref is readonly outside package model`

	account.Parent.Name = "x" // shallow: 参照先は書き込み可能
	account.Parent = p        // want `field Account\.Parent is readonly outside package model`
}

// タグ付き埋め込みフィールド経由で昇格したフィールドへの書き込みも禁止される。
func embeddedInterior(doc *model.Doc) {
	doc.N = 1 // want `field Doc\.Audit is readonly outside package model`
}

// ポインタ経由の構造体丸ごと代入も禁止される。
func wholeStore(user *model.User) {
	*user = model.User{} // want `cannot assign to \*user: field User\.ID is readonly outside package model`
}

// 既存の格納先(スライス要素、フィールド、埋め込み先、配列要素)への丸ごと代入も
// 禁止される。値として readonly フィールドを含む構造体は、入れ子でも同様。
func wholeStoreIntoStorage(users []model.User, order *model.Order, admin *Admin, team *model.Team, teams []model.Team) {
	users[0] = model.User{}        // want `cannot assign to users\[0\]: field User\.ID is readonly outside package model`
	order.User = model.User{}      // want `cannot assign to order\.User: field User\.ID is readonly outside package model`
	*order = model.Order{}         // want `cannot assign to \*order: field User\.ID is readonly outside package model`
	*admin = Admin{}               // want `cannot assign to \*admin: field User\.ID is readonly outside package model`
	team.Members[1] = model.User{} // want `cannot assign to team\.Members\[1\]: field User\.ID is readonly outside package model`
	teams[0] = model.Team{}        // want `cannot assign to teams\[0\]: field User\.ID is readonly outside package model`
}

// 裸の変数への代入は初期化とみなして許可される。参照(ポインタ・スライス・マップ)の
// 差し替えは、参照先の中身を上書きしないので許可される。
func wholeStoreAllowed(team *model.Team, user *model.User, ptrs []*model.User) {
	u := model.User{}
	u = model.User{ID: "x"}
	_ = u
	team.Guests = nil
	team.Owner = user
	ptrs[0] = user
}

// マップ要素への丸ごと代入は、キーが指す値の差し替えなので許可される。
// マップの値はアドレスを取れず、その場での上書きを観測できないため。
func mapEntry(users []model.User, m map[string]model.User) map[string]model.User {
	byID := map[string]model.User{}
	for _, u := range users {
		byID[u.ID] = u
	}
	m["k"] = model.User{}
	return byID
}

// range 節での代入も禁止される。
func rangeAssign(counter *model.Counter, xs []int) {
	for counter.Value = range xs { // want `field Counter\.Value is readonly outside package model`
	}
}

// immutable フィールドは外部パッケージからも再代入できない。
func renumber(inv *model.Invoice) {
	inv.Number = "INV-3"   // want `field Invoice\.Number is immutable`
	*inv = model.Invoice{} // want `cannot assign to \*inv: field Invoice\.Number is immutable`
}

// 中身を書き換える組み込み関数(delete / clear / copy)も禁止される。
func builtins(account *model.Account, src []string) {
	delete(account.Meta, "k")      // want `field Account\.Meta is readonly outside package model`
	clear(account.Meta)            // want `field Account\.Meta is readonly outside package model`
	clear(account.Items)           // want `field Account\.Items is readonly outside package model`
	copy(account.Items, src)       // want `field Account\.Items is readonly outside package model`
	copy(account.Items[1:], src)   // want `field Account\.Items is readonly outside package model`
	account.Items[1:][0] = "x"     // want `field Account\.Items is readonly outside package model`
	copy(src, account.Items)       // 読み取り側は許可
	_ = append(account.Items, "x") // 対象外(制限事項: 余剰容量があれば backing array に書く)
}

// 同名のユーザー定義関数は組み込みではないので対象外。
func shadowedBuiltin(account *model.Account) {
	delete := func(m map[string]string, k string) { _ = m[k] }
	delete(account.Meta, "k")
}

// shallow なら組み込み関数による中身の書き換えも許可される。
func builtinsShallow(cart *model.Cart, src []string) {
	clear(cart.Lines)
	copy(cart.Lines, src)
}

// immutable でも composite literal による生成とタグなしフィールドへの代入は許可される。
func newInvoice() *model.Invoice {
	inv := &model.Invoice{Number: "INV-1"}
	inv.Note = "ok"
	return inv
}

// shallow は中身への書き込みを許可し、フィールド自体の再代入のみ禁止する。
func shallow(cart *model.Cart, w *model.Wrap) {
	cart.Lines[0] = "ok"
	cart.Owner.Name = "ok"
	w.Name = "ok" // 昇格フィールド経由の中身書き込みも許可

	cart.Lines = nil             // want `field Cart\.Lines is readonly outside package model`
	cart.Owner = model.Profile{} // want `field Cart\.Owner is readonly outside package model`
	w.Profile = model.Profile{}  // want `field Wrap\.Profile is readonly outside package model`
}

// 未知のタグ値は宣言時に報告される。
type config struct {
	Mode string `readonly:"writable"` // want `invalid readonly tag value "writable" \(valid values: "external", "immutable", "-"\)`
}

// ブランクフィールドの既定タグは、他のフィールドに個別のタグがない場合に適用される。
type Wrapper struct {
	model.Tenant
}

func structDefault(ev *model.Event, t *model.Tenant, w *Wrapper) {
	ev.ID = "x"         // want `field Event\.ID is immutable`
	ev.At = 1           // want `field Event\.At is immutable`
	ev.Note = "x"       // readonly:"-" で既定から外れている
	*ev = model.Event{} // want `cannot assign to \*ev: field Event\.ID is immutable`
	_ = model.Event{ID: "x", At: 1}

	t.ID = "x"       // want `field Tenant\.ID is readonly outside package model`
	t.Tags[0] = "ok" // 既定が shallow なので中身は書き込み可能
	t.Tags = nil     // want `field Tenant\.Tags is readonly outside package model`
	t.Owner = "x"    // want `field Tenant\.Owner is immutable`
	w.ID = "x"       // want `field Tenant\.ID is readonly outside package model`
}

// ブランクフィールドのタグの誤用は宣言時に報告される。
type badDefault struct {
	_ struct{} `readonly:"-"` // want `readonly tag value "-" is not allowed on a blank field \(valid values: "external", "immutable"\)`
}

type badOptOut struct {
	Mode string `readonly:"-,shallow"` // want `readonly tag value "-" takes no options`
}

type duplicateDefault struct {
	_    struct{} `readonly:"external"`
	_    struct{} `readonly:"immutable"` // want `duplicate readonly default: only the first blank field's tag applies`
	Mode string
}

// 既定のない構造体での readonly:"-" は何もしないが、エラーにもならない。
type plainOptOut struct {
	Mode string `readonly:"-"`
}

func optOutNoDefault(p *plainOptOut, d *duplicateDefault) {
	p.Mode = "ok"
	d.Mode = "ok" // 最初の既定(external)が効く。2 つ目の immutable なら報告されるはず
}

// ブランクフィールドの既定に無効な値を書くと、候補に "-" は出ない。
type badDefaultValue struct {
	_ struct{} `readonly:"externl"` // want `invalid readonly tag value "externl" \(valid values: "external", "immutable"\)`
}

// 複数名のフィールド宣言にブランクが含まれると、タグが構造体の既定になってしまうので拒否する。
type multiName struct {
	A, _ int `readonly:"external"` // want `readonly tag on a field list with a blank name would set the struct default; declare the blank field on its own`
	Name string
}

func multiNameNotDefault(m *multiName) {
	m.Name = "ok" // 型情報上は既定(external)が効くが、宣言側の報告で気づける。同一パッケージなので許可
}

// 型パラメータのマップ要素への丸ごと代入も、マップと同じく許可される。
type userMap interface{ ~map[string]model.User }

func genericMap[M ~map[string]model.User](m M) {
	m["k"] = model.User{}
}

func genericMapNamed[M userMap](m M) {
	m["k"] = model.User{}
}

func genericMapEmbedded[M interface {
	userMap
	fmt.Stringer
}](m M) {
	m["k"] = model.User{}
}

// 型パラメータのスライス要素は引き続き報告される。
func genericSlice[S ~[]model.User](s S) {
	s[0] = model.User{} // want `cannot assign to s\[0\]: field User\.ID is readonly outside package model`
}

// 未知のオプションも宣言時に報告される。
type config2 struct {
	Mode string `readonly:"external,shalow"` // want `invalid readonly tag option "shalow" \(valid options: "shallow"\)`
}

// 大文字小文字だけが異なるキーも宣言時に報告される。他のキーは対象外。
type config3 struct {
	Mode  string `ReadOnly:"external"`               // want `unrecognized struct tag key "ReadOnly" \(did you mean "readonly"\?\)`
	Other string `json:"other" READONLY:"immutable"` // want `unrecognized struct tag key "READONLY" \(did you mean "readonly"\?\)`
	Plain string `json:"plain" readonlyx:"x"`
}

// Struct Literal による初期化は許可される。
func create(id, tenantID string) model.User {
	return model.User{
		ID:       id,
		TenantID: tenantID,
		Status:   model.StatusActive,
	}
}

// タグのないフィールドへの代入は許可される。
func untagged(user *model.User) {
	user.Name = "ok"
}

// 読み取りは許可される。
func read(user model.User) model.Status {
	return user.Status
}

// 同名フィールドでもタグがなければ対象外。
type localUser struct {
	Status model.Status
}

func localAssign(u *localUser) {
	u.Status = model.StatusDeleted
}
