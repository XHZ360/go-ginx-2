package session

// VirtualSession is a provider session implemented inside the server runtime.
type VirtualSession interface {
	StreamOpener
}

// SessionRegistry is the lifecycle boundary used by virtual sessions.
type SessionRegistry interface {
	Register(input RegisterInput) (Session, *Session, error)
	Close(sessionID string) (Session, bool, error)
}

var _ SessionRegistry = (*Manager)(nil)
