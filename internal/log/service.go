package log

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/HammerMeetNail/nabu/internal/audit"
	"github.com/HammerMeetNail/nabu/internal/chore"
)

var ErrNotFound = errors.New("log entry not found")

// ErrInvalidInput marks request data that failed per-field validation
// (audit finding #10). Handlers map it to 400 with the wrapped message.
var ErrInvalidInput = errors.New("invalid input")

// Per-field input caps (audit finding #10). The server is the authority even
// though the clients constrain these fields in the UI.
const (
	maxNoteRunes       = 2000
	maxTitleRunes      = 120
	maxSubjectRunes    = 30
	maxIndicators      = 8
	maxIndicatorRunes  = 30
	maxVolumeML        = 100000
	maxSlotHour        = 23
	maxRating          = 50 // tenths of a star
	maxDurationSeconds = 86400
)

type Service struct {
	store       Store
	chores      *chore.Service
	memberships chore.MembershipReader
	now         func() time.Time
	auditLogger audit.Logger
}

func NewService(store Store) *Service {
	return &Service{
		store:       store,
		now:         func() time.Time { return time.Now().UTC() },
		auditLogger: audit.NopLogger{},
	}
}

// SetAuditLogger attaches a sink for chore-log mutation events. A nil logger is
// a no-op (the service keeps its default NopLogger).
func (s *Service) SetAuditLogger(logger audit.Logger) {
	if logger != nil {
		s.auditLogger = logger
	}
}

func (s *Service) logAudit(ctx context.Context, event string, attrs map[string]string) {
	audit.Emit(ctx, s.auditLogger, event, attrs)
}

func idStr(id int64) string { return strconv.FormatInt(id, 10) }

// validateLogInput enforces per-field caps on every create and update path
// (audit finding #10). It rejects with a clear error rather than truncating.
func validateLogInput(title *string, note string, indicators []string, indicatorVolumes map[string]int, slotHour *int, rating *int, durationSeconds *int, subject *string) error {
	if utf8.RuneCountInString(note) > maxNoteRunes {
		return fmt.Errorf("%w: note must be %d characters or fewer", ErrInvalidInput, maxNoteRunes)
	}
	if title != nil && utf8.RuneCountInString(*title) > maxTitleRunes {
		return fmt.Errorf("%w: title must be %d characters or fewer", ErrInvalidInput, maxTitleRunes)
	}
	if subject != nil {
		if utf8.RuneCountInString(*subject) > maxSubjectRunes {
			return fmt.Errorf("%w: subject must be %d characters or fewer", ErrInvalidInput, maxSubjectRunes)
		}
		if strings.ContainsAny(*subject, "\x00\n\r\t") {
			return fmt.Errorf("%w: subject contains invalid characters", ErrInvalidInput)
		}
	}
	if len(indicators) > maxIndicators {
		return fmt.Errorf("%w: too many indicators", ErrInvalidInput)
	}
	for _, label := range indicators {
		if utf8.RuneCountInString(label) == 0 || utf8.RuneCountInString(label) > maxIndicatorRunes {
			return fmt.Errorf("%w: indicators must each be 1-%d characters", ErrInvalidInput, maxIndicatorRunes)
		}
	}
	if len(indicatorVolumes) > maxIndicators {
		return fmt.Errorf("%w: too many indicator volumes", ErrInvalidInput)
	}
	for label, vol := range indicatorVolumes {
		if utf8.RuneCountInString(label) == 0 || utf8.RuneCountInString(label) > maxIndicatorRunes {
			return fmt.Errorf("%w: indicator volume labels must each be 1-%d characters", ErrInvalidInput, maxIndicatorRunes)
		}
		if vol < 0 || vol > maxVolumeML {
			return fmt.Errorf("%w: indicator volumes must be between 0 and %d", ErrInvalidInput, maxVolumeML)
		}
	}
	if slotHour != nil && (*slotHour < 0 || *slotHour > maxSlotHour) {
		return fmt.Errorf("%w: hour must be between 0 and %d", ErrInvalidInput, maxSlotHour)
	}
	if rating != nil && (*rating < 0 || *rating > maxRating) {
		return fmt.Errorf("%w: rating must be between 0 and %d", ErrInvalidInput, maxRating)
	}
	if durationSeconds != nil && (*durationSeconds < 0 || *durationSeconds > maxDurationSeconds) {
		return fmt.Errorf("%w: duration must be between 0 and %d seconds", ErrInvalidInput, maxDurationSeconds)
	}
	return nil
}

func (s *Service) LogChore(ctx context.Context, householdID, userID, choreID int64, title *string, note string, indicators []string, indicatorVolumes map[string]int, date *time.Time, slotHour *int, completedAt *time.Time, volumeML *int, rating *int, durationSeconds *int, subject *string) (ChoreLog, error) {
	if err := validateLogInput(title, note, indicators, indicatorVolumes, slotHour, rating, durationSeconds, subject); err != nil {
		return ChoreLog{}, err
	}
	entry := s.buildLog(householdID, userID, choreID, title, note, indicators, indicatorVolumes, date, slotHour, completedAt, volumeML, rating, durationSeconds, subject)
	created, err := s.store.CreateLog(ctx, entry)
	if err != nil {
		return ChoreLog{}, err
	}
	// The actor (who performed the logging) is the authenticated user in
	// context, NOT necessarily the chore log's UserID — a member can log a
	// chore on behalf of another household member. audit.Emit enriches with
	// the context actor so the audit trail records who did it.
	s.logAudit(ctx, "log.created", map[string]string{
		"household_id": idStr(householdID),
		"log_id":       idStr(created.ID),
		"chore_id":     idStr(choreID),
	})
	return created, nil
}

// buildLog assembles a ChoreLog from the create-log inputs, resolving the
// canonical completion timestamp and log date. Shared by LogChore and
// LogChoreIdempotent.
func (s *Service) buildLog(householdID, userID, choreID int64, title *string, note string, indicators []string, indicatorVolumes map[string]int, date *time.Time, slotHour *int, completedAt *time.Time, volumeML *int, rating *int, durationSeconds *int, subject *string) ChoreLog {
	var logCompletedAt time.Time
	if completedAt != nil {
		logCompletedAt = completedAt.UTC()
	} else if date != nil {
		// Use noon UTC so the timestamp falls clearly within the requested day.
		logCompletedAt = time.Date(date.Year(), date.Month(), date.Day(), 12, 0, 0, 0, time.UTC)
	} else {
		logCompletedAt = s.now()
	}
	if indicators == nil {
		indicators = []string{}
	}
	var logDate *string
	if date != nil {
		d := date.Format("2006-01-02")
		logDate = &d
	}
	return ChoreLog{
		HouseholdID:      householdID,
		UserID:           userID,
		ChoreID:          choreID,
		CompletedAt:      logCompletedAt,
		Title:            title,
		Note:             note,
		Indicators:       indicators,
		IndicatorVolumes: indicatorVolumes,
		SlotHour:         slotHour,
		LogDate:          logDate,
		VolumeML:         volumeML,
		Rating:           rating,
		DurationSeconds:  durationSeconds,
		Subject:          subject,
	}
}

// LogChoreIdempotent creates a log, de-duplicating by a client-generated
// idempotency key so offline replay is safe. Returns (log, created) where
// created is false when an existing log with the same key was returned. When
// key is empty it behaves exactly like LogChore (always creates).
func (s *Service) LogChoreIdempotent(ctx context.Context, input CreateInput) (ChoreLog, bool, error) {
	if err := validateLogInput(input.Title, input.Note, input.Indicators, input.IndicatorVolumes, input.SlotHour, input.Rating, input.DurationSeconds, input.Subject); err != nil {
		return ChoreLog{}, false, err
	}
	if err := s.authorizeSubmission(ctx, input); err != nil {
		return ChoreLog{}, false, err
	}
	if input.MetricUnit != "" {
		if err := validateUnit(input.MetricUnit); err != nil {
			return ChoreLog{}, false, err
		}
	}
	fingerprint, err := input.fingerprint()
	if err != nil {
		return ChoreLog{}, false, err
	}
	if input.IdempotencyKey != "" {
		if existing, err := s.store.FindLogByIdempotencyKey(ctx, input.HouseholdID, input.IdempotencyKey); err != nil {
			return ChoreLog{}, false, err
		} else if existing != nil {
			return s.authorizedReplay(ctx, input, fingerprint, *existing)
		}
	}
	entry := s.buildLog(input.HouseholdID, input.UserID, input.ChoreID, input.Title, input.Note, input.Indicators, input.IndicatorVolumes, input.Date, input.SlotHour, input.CompletedAt, input.VolumeML, input.Rating, input.DurationSeconds, input.Subject)
	entry.MetricUnit, err = s.snapshotUnit(ctx, input.ActorID, input.HouseholdID, input.ChoreID, input.MetricUnit)
	if err != nil {
		return ChoreLog{}, false, err
	}
	entry.IdempotencyKey = input.IdempotencyKey
	entry.IdempotencyActorID = input.ActorID
	entry.IdempotencyHash = fingerprint
	created, err := s.store.CreateLog(ctx, entry)
	if err != nil {
		// Both lookup paths apply identical request binding and current access
		// checks. Never return the winner of a unique-index race unchecked.
		if input.IdempotencyKey != "" {
			if existing, ferr := s.store.FindLogByIdempotencyKey(ctx, input.HouseholdID, input.IdempotencyKey); ferr == nil && existing != nil {
				return s.authorizedReplay(ctx, input, fingerprint, *existing)
			}
		}
		return ChoreLog{}, false, err
	}
	s.logAudit(ctx, "log.created", map[string]string{
		"household_id": idStr(input.HouseholdID),
		"log_id":       idStr(created.ID),
		"chore_id":     idStr(input.ChoreID),
	})
	return created, true, nil
}

func (s *Service) UpdateLog(ctx context.Context, logID int64, householdID int64, title *string, note string, indicators []string, indicatorVolumes map[string]int, volumeML *int, userID *int64, completedAt *time.Time, slotHour *int, logDate *time.Time, rating *int, durationSeconds *int, subject *string, patches ...Patch) error {
	if err := validateLogInput(title, note, indicators, indicatorVolumes, slotHour, rating, durationSeconds, subject); err != nil {
		return err
	}
	log, err := s.store.GetLog(ctx, logID)
	if err != nil {
		return err
	}
	if log.HouseholdID != householdID {
		return errors.New("log does not belong to your household")
	}
	var fields LogFields
	if len(patches) > 0 {
		fields = patches[0].Fields
		if fields == nil {
			fields = LogFields{}
		}
		if fields.includes("completedAt") && completedAt == nil || fields.includes("userId") && userID == nil {
			return fmt.Errorf("%w: userId and completedAt cannot be null", ErrInvalidInput)
		}
		if s.chores != nil {
			if _, err := s.chores.GetVisible(ctx, patches[0].ActorID, householdID, log.ChoreID); err != nil {
				return ErrNotFound
			}
			if userID != nil && *userID != log.UserID {
				if err := s.authorizeSubmission(ctx, CreateInput{HouseholdID: householdID, ActorID: patches[0].ActorID, UserID: *userID, ChoreID: log.ChoreID}); err != nil {
					return err
				}
			}
		}
	}
	if volumeML != nil && (*volumeML < 0 || *volumeML > maxVolumeML) {
		return fmt.Errorf("%w: amount must be between 0 and %d", ErrInvalidInput, maxVolumeML)
	}
	if len(patches) > 0 && fields.includes("metricUnit") {
		if patches[0].MetricUnit == nil {
			return fmt.Errorf("%w: unit cannot be null", ErrInvalidInput)
		}
		if err := validateUnit(*patches[0].MetricUnit); err != nil {
			return err
		}
		log.MetricUnit, err = s.snapshotUnit(ctx, patches[0].ActorID, householdID, log.ChoreID, *patches[0].MetricUnit)
		if err != nil {
			return err
		}
	}
	if log.MetricUnit == "" && ((fields.includes("volumeML") && volumeML != nil) || (fields.includes("indicatorVolumes") && len(indicatorVolumes) > 0)) {
		actorID := log.UserID
		if len(patches) > 0 {
			actorID = patches[0].ActorID
		}
		log.MetricUnit, err = s.snapshotUnit(ctx, actorID, householdID, log.ChoreID, "")
		if err != nil {
			return err
		}
		if fields != nil {
			copyFields := LogFields{}
			for key, value := range fields {
				copyFields[key] = value
			}
			copyFields["snapshotMetricUnit"] = true
			fields = copyFields
		}
	}
	log.Note = note
	log.Title = title
	if indicators == nil {
		indicators = []string{}
	}
	log.Indicators = indicators
	log.IndicatorVolumes = indicatorVolumes
	log.VolumeML = volumeML
	log.Rating = rating
	log.DurationSeconds = durationSeconds
	log.Subject = subject
	if userID != nil {
		log.UserID = *userID
	}
	if completedAt != nil {
		log.CompletedAt = completedAt.UTC()
	}
	log.SlotHour = slotHour
	if fields != nil && fields.includes("date") {
		log.LogDate = nil
	}
	if logDate != nil {
		d := logDate.Format("2006-01-02")
		log.LogDate = &d
	}
	if err := s.store.UpdateLog(ctx, log, fields); err != nil {
		return err
	}
	s.logAudit(ctx, "log.updated", map[string]string{
		"household_id": idStr(householdID),
		"log_id":       idStr(logID),
	})
	return nil
}

func (s *Service) UndoLog(ctx context.Context, householdID, logID int64) error {
	log, err := s.store.GetLog(ctx, logID)
	if err != nil {
		return err
	}
	if log.HouseholdID != householdID {
		return errors.New("can only undo logs in your own household")
	}
	if err := s.store.DeleteLog(ctx, logID); err != nil {
		return err
	}
	s.logAudit(ctx, "log.deleted", map[string]string{
		"household_id": idStr(householdID),
		"log_id":       idStr(logID),
	})
	return nil
}

func (s *Service) GetTodayLogs(ctx context.Context, householdID int64) ([]ChoreLog, error) {
	return s.store.ListLogs(ctx, householdID, s.today())
}

func (s *Service) GetDayLogs(ctx context.Context, householdID int64, date time.Time) ([]ChoreLog, error) {
	return s.store.ListLogs(ctx, householdID, date)
}

func (s *Service) GetWeekLogs(ctx context.Context, householdID int64, start time.Time) ([]ChoreLog, error) {
	end := start.AddDate(0, 0, 7)
	return s.store.ListLogsRange(ctx, householdID, start, end)
}

func (s *Service) GetMonthLogs(ctx context.Context, householdID int64, year int, month time.Month) ([]ChoreLog, error) {
	start := time.Date(year, month, 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 1, 0)
	return s.store.ListLogsRange(ctx, householdID, start, end)
}

// GetLogsInRange returns the household's logs with completed_at in
// [start, end). Used by the CSV export.
func (s *Service) GetLogsInRange(ctx context.Context, householdID int64, start, end time.Time) ([]ChoreLog, error) {
	return s.store.ListLogsRange(ctx, householdID, start, end)
}

func (s *Service) GetDailySummary(ctx context.Context, householdID int64, date time.Time) (DailySummary, error) {
	logs, err := s.store.ListLogs(ctx, householdID, date)
	if err != nil {
		return DailySummary{}, err
	}
	return s.DailySummaryFromLogs(date, logs), nil
}

func (s *Service) DailySummaryFromLogs(date time.Time, logs []ChoreLog) DailySummary {
	summary := DailySummary{
		Date:        date.Format("2006-01-02"),
		TotalChores: len(logs),
		ChoresDone:  len(logs),
		ByUser:      map[int64]int{},
		ByCategory:  map[string]int{},
	}
	for _, l := range logs {
		summary.ByUser[l.UserID]++
	}
	return summary
}

func (s *Service) today() time.Time {
	now := s.now()
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
}

func (s *Service) GetLog(ctx context.Context, id int64) (ChoreLog, error) {
	return s.store.GetLog(ctx, id)
}

func (s *Service) GetLogForHousehold(ctx context.Context, id, householdID int64) (ChoreLog, error) {
	l, err := s.store.GetLog(ctx, id)
	if err != nil {
		return ChoreLog{}, err
	}
	if l.HouseholdID != householdID {
		return ChoreLog{}, ErrNotFound
	}
	return l, nil
}

func (s *Service) LatestPerChore(ctx context.Context, householdID int64) (map[int64]ChoreLog, error) {
	return s.store.LatestPerChore(ctx, householdID)
}

func (s *Service) GetHistoryLogs(ctx context.Context, householdID int64, start, end time.Time) ([]ChoreLog, bool, error) {
	return s.store.HistoryLogs(ctx, householdID, start, end)
}

// SearchHistoryLogs returns up to limit logs across all household history
// whose note or title matches query (case-insensitive substring).
func (s *Service) SearchHistoryLogs(ctx context.Context, householdID int64, query string, limit int) ([]ChoreLog, error) {
	return s.store.SearchHistoryLogs(ctx, householdID, query, limit)
}
