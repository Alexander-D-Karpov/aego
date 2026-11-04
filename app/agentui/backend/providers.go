package backend

import "fmt"

// ForProvider selects a CLI without starting a process or reading its credentials.
func ForProvider(name string) (Backend, error) {
	switch name {
	case "", "claude":
		return Claude{}, nil
	case "codex":
		return Codex{}, nil
	default:
		return nil, fmt.Errorf("unknown agent provider %q", name)
	}
}
