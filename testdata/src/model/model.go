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

type Invoice struct {
	Number string `readonly:"immutable"`
	Note   string
}

// immutable フィールドは composite literal による生成でのみ値を設定できる。
func NewInvoice(number string) *Invoice {
	return &Invoice{Number: number}
}

// immutable フィールドは定義パッケージ内でも再代入できない。
func Renumber(inv *Invoice) {
	inv.Number = "INV-2" // want `field Invoice\.Number is immutable`
}

type Cart struct {
	Lines []string `readonly:"external,shallow"`
	Owner Profile  `readonly:"external,shallow"`
}

type Wrap struct {
	Profile `readonly:"external,shallow"`
}

type Snapshot struct {
	Data []byte `readonly:"immutable,shallow"`
}

// shallow な immutable は中身の書き込みを許可するが、再代入は定義パッケージ内でも不可。
func Patch(s *Snapshot) {
	s.Data[0] = 0
	s.Data = nil // want `field Snapshot\.Data is immutable`
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
