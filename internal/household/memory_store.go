package household

import (
	"context"
	"sync"
	"time"
)

type MemoryStore struct {
	mu           sync.RWMutex
	idSeq        int64
	households   map[int64]Household
	members      map[int64][]Member
	userHH       map[int64]int64
	invites      map[int64]Invite
	inviteByCode map[string]Invite
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		households:   map[int64]Household{},
		members:      map[int64][]Member{},
		userHH:       map[int64]int64{},
		invites:      map[int64]Invite{},
		inviteByCode: map[string]Invite{},
	}
}

func (s *MemoryStore) nextID() int64 {
	s.idSeq++
	return s.idSeq
}

func (s *MemoryStore) CreateHousehold(_ context.Context, name string, ownerID int64) (Household, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	id := s.nextID()
	code := GenerateInviteCode()
	now := time.Now().UTC()
	hh := Household{
		ID:         id,
		Name:       name,
		InviteCode: code,
		CreatedAt:  now,
	}
	s.households[id] = hh
	s.members[id] = []Member{{
		UserID:      ownerID,
		Email:       "",
		DisplayName: "",
		AvatarColor: "#19323C",
		Role:        RoleOwner,
	}}
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
	return s.households[hhID], nil
}

func (s *MemoryStore) UpdateHousehold(_ context.Context, id int64, name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	hh, ok := s.households[id]
	if !ok {
		return ErrNotFound
	}
	hh.Name = name
	s.households[id] = hh
	return nil
}

func (s *MemoryStore) GetMembers(_ context.Context, householdID int64) ([]Member, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	members, ok := s.members[householdID]
	if !ok {
		return nil, ErrNotFound
	}
	result := make([]Member, len(members))
	copy(result, members)
	return result, nil
}

func (s *MemoryStore) AddMember(_ context.Context, householdID, userID int64, role string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.households[householdID]; !ok {
		return ErrNotFound
	}
	if _, ok := s.userHH[userID]; ok {
		return ErrAlreadyMember
	}
	s.members[householdID] = append(s.members[householdID], Member{
		UserID: userID,
		Role:   role,
	})
	s.userHH[userID] = householdID
	return nil
}

func (s *MemoryStore) RemoveMember(_ context.Context, householdID, userID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	members, ok := s.members[householdID]
	if !ok {
		return ErrNotFound
	}
	for i, m := range members {
		if m.UserID == userID {
			s.members[householdID] = append(members[:i], members[i+1:]...)
			delete(s.userHH, userID)
			return nil
		}
	}
	return ErrNotMember
}

func (s *MemoryStore) UpdateMemberRole(_ context.Context, householdID, userID int64, role string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	members, ok := s.members[householdID]
	if !ok {
		return ErrNotFound
	}
	for i, m := range members {
		if m.UserID == userID {
			s.members[householdID][i].Role = role
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
	members, ok := s.members[hhID]
	if !ok {
		return 0, "", ErrNotFound
	}
	for _, m := range members {
		if m.UserID == userID {
			return hhID, m.Role, nil
		}
	}
	return 0, "", ErrNotFound
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
	inv := Invite{
		ID:          id,
		HouseholdID: householdID,
		Code:        code,
		CreatedBy:   createdBy,
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

func (s *MemoryStore) GetInvites(_ context.Context, householdID int64) ([]Invite, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []Invite
	for _, inv := range s.invites {
		if inv.HouseholdID == householdID {
			result = append(result, inv)
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
