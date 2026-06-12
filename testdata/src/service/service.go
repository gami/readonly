package service

import "model"

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

// タグ付き埋め込みフィールド経由で昇格したフィールドへの書き込みも禁止される。
func embeddedInterior(doc *model.Doc) {
	doc.N = 1 // want `field Doc\.Audit is readonly outside package model`
}

// ポインタ経由の構造体丸ごと代入も禁止される。
func wholeStore(user *model.User) {
	*user = model.User{} // want `cannot assign to \*User: field User\.ID is readonly outside package model`
}

// range 節での代入も禁止される。
func rangeAssign(counter *model.Counter, xs []int) {
	for counter.Value = range xs { // want `field Counter\.Value is readonly outside package model`
	}
}

// immutable フィールドは外部パッケージからも再代入できない。
func renumber(inv *model.Invoice) {
	inv.Number = "INV-3"   // want `field Invoice\.Number is immutable`
	*inv = model.Invoice{} // want `cannot assign to \*Invoice: field Invoice\.Number is immutable`
}

// immutable でも composite literal による生成とタグなしフィールドへの代入は許可される。
func newInvoice() *model.Invoice {
	inv := &model.Invoice{Number: "INV-1"}
	inv.Note = "ok"
	return inv
}

// 未知のタグ値は宣言時に報告される。
type config struct {
	Mode string `readonly:"writable"` // want `invalid readonly tag value "writable" \(valid values: "external", "immutable"\)`
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
