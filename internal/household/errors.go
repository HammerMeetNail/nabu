package household

import "errors"

var (
	ErrNotFound       = errors.New("household not found")
	ErrAlreadyMember  = errors.New("user is already a member of a household")
	ErrNotMember      = errors.New("user is not a member of this household")
	ErrInviteNotFound = errors.New("invite not found")
	ErrInviteExpired  = errors.New("invite has expired")
	ErrLastOwner      = errors.New("cannot remove the last owner")
	ErrNotAuthorized  = errors.New("not authorized")
	// ErrInvalidInput marks request data that failed per-field validation
	// (audit finding #10). Handlers map it to 400 with the wrapped message.
	ErrInvalidInput = errors.New("invalid input")
	// ErrAccessSuspended marks that the caller's own membership is disabled.
	// Handlers map it to 403 with a client-visible message.
	ErrAccessSuspended = errors.New("membership access is suspended")
)

// AccessSuspendedError carries the household whose membership was disabled so
// the client can name it: "Your access to <name> is temporarily disabled."
type AccessSuspendedError struct {
	HouseholdName string
}

func (e *AccessSuspendedError) Error() string {
	return ErrAccessSuspended.Error() + ": " + e.HouseholdName
}

func (e *AccessSuspendedError) Unwrap() error { return ErrAccessSuspended }

const MaxMembersPerHousehold = 20
