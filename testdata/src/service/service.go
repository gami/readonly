package service

import "model"

// 外部パッケージからの代入は禁止される。
func direct(user model.User) {
	user.Status = model.StatusDeleted // want `field User\.Status is marked reassign:"internal" and cannot be modified outside package model`
}

// ポインタ経由も禁止される。
func viaPointer(userPtr *model.User) {
	userPtr.Status = model.StatusDeleted // want `field User\.Status is marked reassign:"internal" and cannot be modified outside package model`
}

// ネストしたアクセスも禁止される。
func nested(order *model.Order) {
	order.User.Status = model.StatusDeleted // want `field User\.Status is marked reassign:"internal" and cannot be modified outside package model`
}

// スライス要素も禁止される。
func sliceElem(users []model.User, i int) {
	users[i].Status = model.StatusDeleted // want `field User\.Status is marked reassign:"internal" and cannot be modified outside package model`
}

// 多重代入の各左辺もそれぞれ検査される。
func multiAssign(user *model.User) {
	user.ID, user.TenantID = "a", "b" // want `field User\.ID is marked reassign:"internal" and cannot be modified outside package model` `field User\.TenantID is marked reassign:"internal" and cannot be modified outside package model`
}

// 複合代入も禁止される。
func compound(user *model.User) {
	user.TenantID += "-suffix" // want `field User\.TenantID is marked reassign:"internal" and cannot be modified outside package model`
}

// 埋め込みで昇格したフィールドへの代入も禁止される。
type Admin struct {
	model.User
}

func promoted(admin *Admin) {
	admin.Status = model.StatusDeleted // want `field User\.Status is marked reassign:"internal" and cannot be modified outside package model`
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
