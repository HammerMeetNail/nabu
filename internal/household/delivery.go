package household

import "context"

// WithMember linearizes a bounded delivery with removal/role changes. Once a
// removal commits, no later delivery can use a previously resolved membership.
// Providers may still deliver messages they accepted before revocation.
func WithMember(ctx context.Context, store Store, userID, householdID int64, deliver func(string) error) error {
	if guarded, ok := store.(interface {
		WithMember(context.Context, int64, int64, func(string) error) error
	}); ok {
		return guarded.WithMember(ctx, userID, householdID, deliver)
	}
	role, err := store.GetMembershipForHousehold(ctx, userID, householdID)
	if err != nil {
		return err
	}
	return deliver(role)
}

func (s *MemoryStore) WithMember(ctx context.Context, userID, householdID int64, deliver func(string) error) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	for _, member := range s.userHouseholds[userID] {
		if member.ID == householdID {
			return deliver(member.Role)
		}
	}
	return ErrNotMember
}

func (s *PostgresStore) WithMember(ctx context.Context, userID, householdID int64, deliver func(string) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	// Negative keys are reserved for household delivery/revocation locks.
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock_shared($1)`, -householdID); err != nil {
		return err
	}
	current := &PostgresStore{db: s.db, q: tx, tx: tx}
	role, err := current.GetMembershipForHousehold(ctx, userID, householdID)
	if err != nil {
		return err
	}
	if err := deliver(role); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *PostgresStore) lockHousehold(ctx context.Context, householdID int64) error {
	_, err := s.q.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, -householdID)
	return err
}
