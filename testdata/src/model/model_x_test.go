package model_test

import "model"

// 定義パッケージ自身の external test package からの代入は許可される。
func fixture() model.User {
	u := model.User{}
	u.Status = model.StatusDeleted
	return u
}
