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
)

const MaxMembersPerHousehold = 20
