package backup

import "context"

// Repo is a repository as reported by a forge: where it lives in the forge's
// namespace, and whether it's archived or carries no commits yet.
type Repo struct {
	Path     string
	Archived bool
	Empty    bool
}

// State selects which repositories a Lister returns.
type State int

// The states a Lister can be asked to filter on.
const (
	StateAll State = iota
	StateActive
	StateArchived
)

func (s State) String() string {
	switch s {
	case StateActive:
		return "active"
	case StateArchived:
		return "archived"
	default:
		return "all"
	}
}

// Lister lists the repositories a forge holds, filtered by state.
type Lister interface {
	ListRepos(ctx context.Context, state State) ([]Repo, error)
}
