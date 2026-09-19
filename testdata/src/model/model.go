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

type Team struct {
	Lead    User
	Members [2]User
	Guests  []User
	Owner   *User
}

type Counter struct {
	Value int `readonly:"external"`
}

type Profile struct {
	Name string
}

// SetName はポインタレシーバなので、呼び出しは暗黙のアドレス取得になる。
func (p *Profile) SetName(name string) { p.Name = name }

// Display は値レシーバなので、呼び出しはコピーに対して行われる。
func (p Profile) Display() string { return p.Name }

type Account struct {
	Profile Profile           `readonly:"external"`
	Items   []string          `readonly:"external"`
	Meta    map[string]string `readonly:"external"`
	Ref     *Profile          `readonly:"external"`
	Parent  *Profile          `readonly:"external,shallow"`
}

type Audit struct {
	N int
}

func (a *Audit) Touch() { a.N++ }

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

// ブランクフィールドのタグは、その構造体の全フィールドの既定になる。
type Event struct {
	_ struct{} `readonly:"immutable"`

	ID   string
	At   int
	Note string `readonly:"-"` // 既定から外す
}

type Tenant struct {
	_ struct{} `readonly:"external,shallow"`

	ID    string
	Tags  []string
	Owner string `readonly:"immutable"` // フィールド自身のタグが優先
}

// 既定が immutable なら定義パッケージ内でも再代入できない。external なら可。
func Touch(ev *Event, t *Tenant) {
	ev.ID = "x" // want `field Event\.ID is immutable`
	ev.Note = "ok"
	t.ID = "ok"
	t.Owner = "x" // want `field Tenant\.Owner is immutable`
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

// 同一パッケージ内では丸ごと代入も組み込み関数による中身の書き換えも許可される。
func Reset(u *User, users []User, a *Account) {
	*u = User{}
	users[0] = User{}
	delete(a.Meta, "k")
	clear(a.Items)
}
