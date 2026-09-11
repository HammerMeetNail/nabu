package log

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/HammerMeetNail/nabu/internal/chore"
)

var ErrIdempotencyConflict = errors.New("idempotency key belongs to another submission")

// CreateInput describes the original logical submission, including follow-up
// effects. Its digest remains unchanged when the resulting log is later edited.
type CreateInput struct {
	MetricUnit                                  string `json:",omitempty"`
	HouseholdID, ActorID, UserID, ChoreID       int64
	Title, Subject                              *string
	Note                                        string
	Indicators                                  []string
	IndicatorVolumes                            map[string]int
	Date, CompletedAt                           *time.Time
	SlotHour, VolumeML, Rating, DurationSeconds *int
	IdempotencyKey                              string `json:"-"`
	FollowUpMinutes                             int
	FollowUpTime                                string
}

func (in CreateInput) fingerprint() (string, error) {
	if in.Indicators == nil {
		in.Indicators = []string{}
	}
	if in.IndicatorVolumes == nil {
		in.IndicatorVolumes = map[string]int{}
	}
	if in.CompletedAt != nil {
		t := in.CompletedAt.UTC()
		in.CompletedAt = &t
	}
	if in.Date != nil {
		t := time.Date(in.Date.Year(), in.Date.Month(), in.Date.Day(), 0, 0, 0, 0, time.UTC)
		in.Date = &t
	}
	b, err := json.Marshal(in)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(b)
	return hex.EncodeToString(hash[:]), nil
}

func (s *Service) WithAccess(cs chore.Store, memberships chore.MembershipReader) *Service {
	if cs != nil {
		s.chores = chore.NewService(cs).WithMemberships(memberships)
	}
	s.memberships = memberships
	return s
}

func (s *Service) authorizeSubmission(ctx context.Context, in CreateInput) error {
	if s.chores == nil {
		return nil // Internal log-store operations without a client viewer.
	}
	if s.memberships == nil || in.ActorID <= 0 {
		return ErrNotFound
	}
	if _, err := s.memberships.GetMembershipForHousehold(ctx, in.ActorID, in.HouseholdID); err != nil {
		return err
	}
	c, err := s.chores.GetVisible(ctx, in.ActorID, in.HouseholdID, in.ChoreID)
	if errors.Is(err, chore.ErrNotFound) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if _, err := s.memberships.GetMembershipForHousehold(ctx, in.UserID, in.HouseholdID); err != nil {
		return err
	}
	if c.Visibility == chore.VisibilityAdmins {
		if err := s.chores.RequireAdmin(ctx, in.UserID, in.HouseholdID); err != nil {
			return ErrNotFound
		}
	}
	return nil
}

func (s *Service) authorizedReplay(ctx context.Context, in CreateInput, fingerprint string, existing ChoreLog) (ChoreLog, bool, error) {
	// Pre-migration records have no trustworthy actor/digest. Refuse to replay
	// them rather than infer their original request from mutable log contents.
	if existing.HouseholdID != in.HouseholdID || existing.ChoreID != in.ChoreID || existing.IdempotencyActorID != in.ActorID || existing.IdempotencyHash == "" || subtle.ConstantTimeCompare([]byte(existing.IdempotencyHash), []byte(fingerprint)) != 1 {
		return ChoreLog{}, false, ErrIdempotencyConflict
	}
	if s.chores != nil {
		if _, err := s.chores.GetVisible(ctx, in.ActorID, existing.HouseholdID, existing.ChoreID); err != nil {
			return ChoreLog{}, false, ErrNotFound
		}
	}
	return existing, false, nil
}
