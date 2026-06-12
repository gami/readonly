package model

type Status string

const (
	StatusActive  Status = "active"
	StatusDeleted Status = "deleted"
)

type User struct {
	ID       string `readonly:"external"`
	TenantID string `readonly:"external"`
	Status   Status `readonly:"external"`

	Name string
}

type Order struct {
	User User
}

type Counter struct {
	Value int `readonly:"external"`
}

type Profile struct {
	Name string
}

type Account struct {
	Profile Profile           `readonly:"external"`
	Items   []string          `readonly:"external"`
	Meta    map[string]string `readonly:"external"`
}

type Audit struct {
	N int
}

type Doc struct {
	Audit `readonly:"external"`
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
