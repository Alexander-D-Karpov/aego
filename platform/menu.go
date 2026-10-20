package platform

type MenuRole uint8

const (
	RoleNone MenuRole = iota
	RoleHide
	RoleHideOthers
	RoleShowAll
)

type MenuItem struct {
	ID        int
	Label     string
	Shortcut  string
	Enabled   bool
	Checked   bool
	Separator bool
	Role      MenuRole
	Items     []MenuItem
}

type Menu struct {
	Title string
	Items []MenuItem
}

type MenuBar interface {
	SetMenuBar(app string, menus []Menu)
	MenuClicks(dst []int) []int
}
