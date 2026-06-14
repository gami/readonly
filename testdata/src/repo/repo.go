package repo

import "model"

// 非テストファイルでの書き込みは allow-all-test-files でも検出される。
func Save(u *model.User) {
	u.Status = model.StatusDeleted // want `field User\.Status is readonly outside package model`
}
