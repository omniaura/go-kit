package errs

// Action is what the person on the other end can do about an error. Clients
// turn it into a button ("Sign in", "Add credits"); when there is nothing they
// can do, leave it empty and let the client offer to report the problem.
//
// Actions are part of the wire contract: lower_snake_case, stable once shipped.
// The constants below cover the common cases; services may declare their own.
type Action string

const (
	// ActionNone means the user cannot fix this; the client should offer to
	// report it (and the Ref identifies it).
	ActionNone Action = ""
	// ActionRetry means trying the same thing again is expected to work.
	ActionRetry Action = "retry"
	// ActionWait means retry after the RetryAfter delay, not immediately.
	ActionWait Action = "wait"
	// ActionSignIn means the session is missing or expired.
	ActionSignIn Action = "sign_in"
	// ActionFixInput means the request itself must change; see Fields.
	ActionFixInput Action = "fix_input"
	// ActionUpgrade means the current plan does not include this.
	ActionUpgrade Action = "upgrade"
	// ActionAddCredits means the balance is too low.
	ActionAddCredits Action = "add_credits"
	// ActionReconnect means a linked account or integration must be re-authorized.
	ActionReconnect Action = "reconnect"
	// ActionUpdateSettings means a saved setting (a key, a preference) is invalid.
	ActionUpdateSettings Action = "update_settings"
	// ActionRequestAccess means someone else has to grant permission.
	ActionRequestAccess Action = "request_access"
	// ActionContactSupport means a person on the team has to intervene.
	ActionContactSupport Action = "contact_support"
)
