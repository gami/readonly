package repo

import "model"

// repository テストでの fixture 差し替えは allow-all-test-files で許可される。
func fixture() model.User {
	u := model.User{}
	u.Status = model.StatusDeleted
	u.TenantID = "other"
	return u
}
