package model

type Status string

const (
	StatusActive  Status = "active"
	StatusDeleted Status = "deleted"
)

type User struct {
	ID       string `reassign:"internal"`
	TenantID string `reassign:"internal"`
	Status   Status `reassign:"internal"`

	Name string
}

type Order struct {
	User User
}

// 同一パッケージ内からの代入は許可される。
func (u *User) ChangeStatus(status Status) {
	u.Status = status
}

func (u *User) Activate() {
	u.Status = StatusActive
}

func New(id, tenantID string) *User {
	u := &User{}
	u.ID = id
	u.TenantID = tenantID
	u.Status = StatusActive
	return u
}
