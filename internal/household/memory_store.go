package household

import (
	"context"
	"github.com/HammerMeetNail/nabu/internal/readlimit"
	"maps"
	"slices"
	"sync"
	"time"
)

type MemoryStore struct {
	mu                sync.RWMutex
	inTransaction     bool
	userExists        func(context.Context, int64) error
	onMembership      func(int64, int64, bool)
	membershipChanges []membershipChange
	idSeq             int64
	households        map[int64]Household
	// userHH maps userID -> active householdID
	userHH map[int64]int64
	// userHouseholds maps userID -> []HouseholdWithRole
	userHouseholds map[int64][]HouseholdWithRole
	invites        map[int64]Invite
	inviteByCode   map[string]Invite
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		households:     map[int64]Household{},
		userHH:         map[int64]int64{},
		userHouseholds: map[int64][]HouseholdWithRole{},
		invites:        map[int64]Invite{},
		inviteByCode:   map[string]Invite{},
	}
}

func (s *MemoryStore) InTransaction(ctx context.Context, fn func(Store) error) error {
	if s.inTransaction {
		return fn(s)
	}
	tx, commit, unlock := s.PrepareTransaction()
	defer unlock()
	if err := fn(tx); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	commit()
	return nil
}

func (s *MemoryStore) PrepareTransaction() (Store, func(), func()) {
	s.mu.Lock()
	memberships := make(map[int64][]HouseholdWithRole, len(s.userHouseholds))
	for userID, entries := range s.userHouseholds {
		memberships[userID] = slices.Clone(entries)
	}
	tx := &MemoryStore{inTransaction: true, userExists: s.userExists, onMembership: s.onMembership, idSeq: s.idSeq, households: maps.Clone(s.households),
		userHH: maps.Clone(s.userHH), userHouseholds: memberships, invites: maps.Clone(s.invites), inviteByCode: maps.Clone(s.inviteByCode)}
	commit := func() {
		s.idSeq, s.households, s.userHH, s.userHouseholds = tx.idSeq, tx.households, tx.userHH, tx.userHouseholds
		s.invites, s.inviteByCode = tx.invites, tx.inviteByCode
		for _, change := range tx.membershipChanges {
			if s.onMembership != nil {
				s.onMembership(change.userID, change.householdID, change.member)
			}
		}
	}
	return tx, commit, s.mu.Unlock
}

func (s *MemoryStore) RevokeInvites(_ context.Context, householdID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.revokeInvites(householdID)
}

func (s *MemoryStore) revokeInvites(householdID int64) error {
	hh, ok := s.households[householdID]
	if !ok {
		return ErrNotFound
	}
	hh.InviteCode = GenerateInviteCode()
	s.households[householdID] = hh
	for id, invite := range s.invites {
		if invite.HouseholdID == householdID {
			delete(s.invites, id)
			delete(s.inviteByCode, invite.Code)
		}
	}
	return nil
}

func (s *MemoryStore) nextID() int64 {
	s.idSeq++
	return s.idSeq
}

func (s *MemoryStore) CreateHousehold(ctx context.Context, name, initials string, ownerID int64) (Household, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.userExists != nil {
		if err := s.userExists(ctx, ownerID); err != nil {
			return Household{}, err
		}
	}
	id := s.nextID()
	code := GenerateInviteCode()
	now := time.Now().UTC()
	hh := Household{
		ID:         id,
		Name:       name,
		Initials:   initials,
		InviteCode: code,
		CreatedAt:  now,
	}
	s.households[id] = hh
	// Add to user_households
	s.userHouseholds[ownerID] = append(s.userHouseholds[ownerID], HouseholdWithRole{
		ID:       id,
		Name:     name,
		Initials: initials,
		Role:     RoleOwner,
	})
	s.userHH[ownerID] = id
	return hh, nil
}

func (s *MemoryStore) GetHousehold(_ context.Context, id int64) (Household, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	hh, ok := s.households[id]
	if !ok {
		return Household{}, ErrNotFound
	}
	return hh, nil
}

func (s *MemoryStore) GetUserHousehold(_ context.Context, userID int64) (Household, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	hhID, ok := s.userHH[userID]
	if !ok {
		return Household{}, ErrNotFound
	}
	hh, ok := s.households[hhID]
	if !ok {
		return Household{}, ErrNotFound
	}
	return hh, nil
}

func (s *MemoryStore) UpdateHousehold(_ context.Context, id int64, name, initials string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	hh, ok := s.households[id]
	if !ok {
		return ErrNotFound
	}
	hh.Name = name
	hh.Initials = initials
	s.households[id] = hh
	// Update name/initials in all userHouseholds entries
	for uid, hhList := range s.userHouseholds {
		for i, h := range hhList {
			if h.ID == id {
				s.userHouseholds[uid][i].Name = name
				s.userHouseholds[uid][i].Initials = initials
			}
		}
	}
	return nil
}

func (s *MemoryStore) DeleteHousehold(_ context.Context, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.households[id]; !ok {
		return ErrNotFound
	}
	delete(s.households, id)
	for uid, hhID := range s.userHH {
		if hhID == id {
			delete(s.userHH, uid)
		}
	}
	for uid, hhList := range s.userHouseholds {
		kept := hhList[:0]
		for _, h := range hhList {
			if h.ID != id {
				kept = append(kept, h)
			}
		}
		if len(kept) == 0 {
			delete(s.userHouseholds, uid)
		} else {
			s.userHouseholds[uid] = kept
		}
	}
	for inviteID, invite := range s.invites {
		if invite.HouseholdID == id {
			delete(s.invites, inviteID)
			delete(s.inviteByCode, invite.Code)
		}
	}
	return nil
}

func (s *MemoryStore) GetMembers(ctx context.Context, householdID int64) ([]Member, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var members []Member
	for userID, hhList := range s.userHouseholds {
		for _, h := range hhList {
			if h.ID == householdID {
				members = append(members, Member{
					UserID: userID,
					Role:   h.Role,
				})
				if err := readlimit.Check(ctx, len(members)); err != nil {
					return nil, err
				}
				break
			}
		}
	}
	if len(members) == 0 {
		return nil, ErrNotFound
	}
	return members, nil
}

func (s *MemoryStore) AddMember(ctx context.Context, householdID, userID int64, role string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.userExists != nil {
		if err := s.userExists(ctx, userID); err != nil {
			return err
		}
	}
	hh, ok := s.households[householdID]
	if !ok {
		return ErrNotFound
	}
	// Check if already a member of this specific household
	for _, h := range s.userHouseholds[userID] {
		if h.ID == householdID {
			return ErrAlreadyMember
		}
	}
	s.userHouseholds[userID] = append(s.userHouseholds[userID], HouseholdWithRole{
		ID:       householdID,
		Name:     hh.Name,
		Initials: hh.Initials,
		Role:     role,
	})
	s.userHH[userID] = householdID
	s.membershipChanged(userID, householdID, true)
	return nil
}

func (s *MemoryStore) RemoveMember(_ context.Context, householdID, userID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	hhList, ok := s.userHouseholds[userID]
	if !ok {
		return ErrNotMember
	}
	found := false
	for i, h := range hhList {
		if h.ID == householdID {
			s.userHouseholds[userID] = append(hhList[:i], hhList[i+1:]...)
			found = true
			break
		}
	}
	if !found {
		return ErrNotMember
	}
	// If removed from active household, switch to another
	if s.userHH[userID] == householdID {
		remaining := s.userHouseholds[userID]
		if len(remaining) > 0 {
			s.userHH[userID] = remaining[0].ID
		} else {
			delete(s.userHH, userID)
		}
	}
	s.membershipChanged(userID, householdID, false)
	return s.revokeInvites(householdID)
}

func (s *MemoryStore) UpdateMemberRole(_ context.Context, householdID, userID int64, role string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	hhList, ok := s.userHouseholds[userID]
	if !ok {
		return ErrNotFound
	}
	for i, h := range hhList {
		if h.ID == householdID {
			s.userHouseholds[userID][i].Role = role
			return nil
		}
	}
	return ErrNotMember
}

func (s *MemoryStore) GetMembership(_ context.Context, userID int64) (int64, string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	hhID, ok := s.userHH[userID]
	if !ok {
		return 0, "", ErrNotFound
	}
	// Find role in user_households for the active household
	for _, h := range s.userHouseholds[userID] {
		if h.ID == hhID {
			return hhID, h.Role, nil
		}
	}
	return 0, "", ErrNotFound
}

func (s *MemoryStore) GetMembershipForHousehold(_ context.Context, userID, householdID int64) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, h := range s.userHouseholds[userID] {
		if h.ID == householdID {
			return h.Role, nil
		}
	}
	return "", ErrNotMember
}

func (s *MemoryStore) ListUserHouseholds(_ context.Context, userID int64) ([]HouseholdWithRole, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	hhList := s.userHouseholds[userID]
	result := make([]HouseholdWithRole, len(hhList))
	copy(result, hhList)
	return result, nil
}

func (s *MemoryStore) SetActiveHousehold(_ context.Context, userID, householdID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Verify user is a member
	found := false
	for _, h := range s.userHouseholds[userID] {
		if h.ID == householdID {
			found = true
			break
		}
	}
	if !found {
		return ErrNotMember
	}
	s.userHH[userID] = householdID
	return nil
}

func (s *MemoryStore) GetHouseholdByInviteCode(_ context.Context, code string) (Household, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, hh := range s.households {
		if hh.InviteCode == code {
			return hh, nil
		}
	}
	return Household{}, ErrInviteNotFound
}

func (s *MemoryStore) CreateInvite(_ context.Context, householdID, createdBy int64, code string, maxUses int) (Invite, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := s.nextID()
	now := time.Now().UTC()
	expires := now.Add(7 * 24 * time.Hour)
	inv := Invite{
		ID:          id,
		HouseholdID: householdID,
		Code:        code,
		CreatedBy:   createdBy,
		ExpiresAt:   &expires,
		MaxUses:     maxUses,
		UsedCount:   0,
		CreatedAt:   now,
	}
	s.invites[id] = inv
	s.inviteByCode[code] = inv
	return inv, nil
}

func (s *MemoryStore) GetInviteByCode(_ context.Context, code string) (Invite, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	inv, ok := s.inviteByCode[code]
	if !ok {
		return Invite{}, ErrInviteNotFound
	}
	if inv.ExpiresAt != nil && time.Now().UTC().After(*inv.ExpiresAt) {
		return Invite{}, ErrInviteExpired
	}
	if inv.MaxUses > 0 && inv.UsedCount >= inv.MaxUses {
		return Invite{}, ErrInviteExpired
	}
	return inv, nil
}

func (s *MemoryStore) GetInviteByID(_ context.Context, id int64) (Invite, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	inv, ok := s.invites[id]
	if !ok {
		return Invite{}, ErrInviteNotFound
	}
	return inv, nil
}

func (s *MemoryStore) GetInvites(ctx context.Context, householdID int64) ([]Invite, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []Invite
	for _, inv := range s.invites {
		if inv.HouseholdID == householdID {
			result = append(result, inv)
			if err := readlimit.Check(ctx, len(result)); err != nil {
				return nil, err
			}
		}
	}
	return result, nil
}

func (s *MemoryStore) UseInvite(_ context.Context, code string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	inv, ok := s.inviteByCode[code]
	if !ok {
		return ErrInviteNotFound
	}
	if inv.ExpiresAt != nil && time.Now().UTC().After(*inv.ExpiresAt) {
		return ErrInviteExpired
	}
	if inv.MaxUses > 0 && inv.UsedCount >= inv.MaxUses {
		return ErrInviteExpired
	}
	inv.UsedCount++
	s.inviteByCode[code] = inv
	s.invites[inv.ID] = inv
	return nil
}

func (s *MemoryStore) DeleteInvite(_ context.Context, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	inv, ok := s.invites[id]
	if ok {
		delete(s.inviteByCode, inv.Code)
	}
	delete(s.invites, id)
	return nil
}

type membershipChange struct {
	userID, householdID int64
	member              bool
}

func (s *MemoryStore) SetMembershipCleanup(fn func(int64, int64, bool)) { s.onMembership = fn }
func (s *MemoryStore) membershipChanged(userID, householdID int64, member bool) {
	if s.inTransaction {
		s.membershipChanges = append(s.membershipChanges, membershipChange{userID, householdID, member})
	} else if s.onMembership != nil {
		s.onMembership(userID, householdID, member)
	}
}
func (s *MemoryStore) RemoveUserInvites(userID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, invite := range s.invites {
		if invite.CreatedBy == userID {
			delete(s.invites, id)
			delete(s.inviteByCode, invite.Code)
		}
	}
}
