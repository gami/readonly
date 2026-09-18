package repo

import "model"

// repository テストでの fixture 差し替えは allow-all-test-files で許可される。
func fixture(users []model.User, a *model.Account) model.User {
	u := model.User{}
	u.Status = model.StatusDeleted
	u.TenantID = "other"
	users[0] = u
	delete(a.Meta, "k")
	return u
}
