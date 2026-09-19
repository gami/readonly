// Package addr は -report-address-of の挙動を検証する。
package addr

import (
	"encoding/json"
	"fmt"

	"model"
)

type resp struct {
	ID *string
}

func ptr[T any](v T) *T { return &v }

// readonly フィールドのアドレスを関数に渡すと報告される。
func args(u *model.User, data []byte) {
	fmt.Sscan("x", &u.ID)           // want `field User\.ID is readonly outside package model \(address passed to a call\)`
	json.Unmarshal(data, &u.Status) // want `field User\.Status is readonly outside package model \(address passed to a call\)`
	fmt.Sscan("x", &u.Name)         // タグなしフィールドは対象外
	fmt.Sscan("x", u.ID)            // 値渡しは対象外
}

// 中身のアドレスも同様に報告される。shallow なら中身は許可される。
func contents(a *model.Account, c *model.Cart) {
	fmt.Sscan("x", &a.Profile.Name) // want `field Account\.Profile is readonly outside package model \(address passed to a call\)`
	fmt.Sscan("x", &a.Items[0])     // want `field Account\.Items is readonly outside package model \(address passed to a call\)`
	fmt.Sscan("x", &c.Lines[0])     // shallow: 中身のアドレスは許可
	fmt.Sscan("x", &c.Lines)        // want `field Cart\.Lines is readonly outside package model \(address passed to a call\)`
}

// 構造体全体のアドレスは対象外。db.Find(&u) のような正当なロードを止めないため。
func whole(u model.User, users []model.User, o *model.Order, data []byte) {
	json.Unmarshal(data, &u)
	json.Unmarshal(data, &users[0])
	json.Unmarshal(data, &o.User)
}

// 読み取り目的でポインタを格納する形は報告されない。
func store(u *model.User) resp {
	r := resp{ID: &u.ID}
	r.ID = &u.ID
	_ = ptr(u.ID)
	_ = append([]*string{}, &u.ID)
	_ = (*string)(&u.ID)
	return r
}

// ポインタレシーバのメソッド呼び出しは暗黙のアドレス取得として報告される。
func receiver(a *model.Account, d *model.Doc, u *model.User, users []model.User, o *model.Order) {
	a.Profile.SetName("x")    // want `field Account\.Profile is readonly outside package model \(pointer receiver call\)`
	(&a.Profile).SetName("x") // want `field Account\.Profile is readonly outside package model \(pointer receiver call\)`
	a.Ref.SetName("x")        // want `field Account\.Ref is readonly outside package model \(pointer receiver call\)`
	a.Parent.SetName("x")     // shallow なポインタ: 参照先への書き込みは許可
	d.Touch()                 // want `field Doc\.Audit is readonly outside package model \(pointer receiver call\)`

	_ = a.Profile.Display() // 値レシーバはコピーに対する呼び出し
	u.Activate()            // 型自身のメソッドは定義パッケージの API なので対象外
	users[0].Activate()
	o.User.Activate() // タグなしフィールド経由も同様
}

// ローカルに保存したポインタを書き込み・関数渡し・メソッド呼び出しに使うと報告される。
func local(u *model.User, a *model.Account) {
	p := &u.ID
	*p = "x"          // want `field User\.ID is readonly outside package model \(written through p, address taken at addr\.go:\d+:\d+\)`
	fmt.Sscan("x", p) // want `field User\.ID is readonly outside package model \(passed to a call through p, address taken at addr\.go:\d+:\d+\)`

	q := &a.Profile
	q.Name = "x"            // want `field Account\.Profile is readonly outside package model \(written through q, address taken at addr\.go:\d+:\d+\)`
	q.SetName("x")          // want `field Account\.Profile is readonly outside package model \(pointer receiver call through q, address taken at addr\.go:\d+:\d+\)`
	fmt.Sscan("x", &q.Name) // want `field Account\.Profile is readonly outside package model \(address passed to a call through q, address taken at addr\.go:\d+:\d+\)`

	var r = &u.Status
	*r = model.StatusDeleted // want `field User\.Status is readonly outside package model \(written through r, address taken at addr\.go:\d+:\d+\)`

	var s *string
	s = &u.TenantID
	*s = "x" // want `field User\.TenantID is readonly outside package model \(written through s, address taken at addr\.go:\d+:\d+\)`

	m := &a.Meta
	delete(*m, "k") // want `field Account\.Meta is readonly outside package model \(written through m, address taken at addr\.go:\d+:\d+\)`
}

// 読み取り専用の使い方は報告されない。
func localRead(u *model.User) resp {
	p := &u.ID
	_ = *p
	r := resp{ID: p}
	p = ptr("x") // 別のポインタへの再代入は書き込みではない
	return r
}

// 制限: コピーしたポインタや返したポインタ経由の書き込みは追跡しない。
func localEscape(u *model.User) *string {
	p := &u.ID
	q := p
	*q = "x"
	return p
}
