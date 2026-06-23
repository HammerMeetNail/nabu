package log

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/HammerMeetNail/nabu/internal/audit"
)

var ErrNotFound = errors.New("log entry not found")

type Service struct {
	store       Store
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

func (s *Service) LogChore(ctx context.Context, householdID, userID, choreID int64, title *string, note string, indicators []string, indicatorVolumes map[string]int, date *time.Time, slotHour *int, completedAt *time.Time, volumeML *int, rating *int) (ChoreLog, error) {
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
	created, err := s.store.CreateLog(ctx, ChoreLog{
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
	})
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

func (s *Service) UpdateLog(ctx context.Context, logID int64, householdID int64, title *string, note string, indicators []string, indicatorVolumes map[string]int, volumeML *int, userID *int64, completedAt *time.Time, slotHour *int, logDate *time.Time, rating *int) error {
	log, err := s.store.GetLog(ctx, logID)
	if err != nil {
		return err
	}
	if log.HouseholdID != householdID {
		return errors.New("log does not belong to your household")
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
	if userID != nil {
		log.UserID = *userID
	}
	if completedAt != nil {
		log.CompletedAt = completedAt.UTC()
	}
	log.SlotHour = slotHour
	if logDate != nil {
		d := logDate.Format("2006-01-02")
		log.LogDate = &d
	}
	if err := s.store.UpdateLog(ctx, log); err != nil {
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

func (s *Service) LatestPerChore(ctx context.Context, householdID int64) (map[int64]ChoreLog, error) {
	return s.store.LatestPerChore(ctx, householdID)
}

func (s *Service) GetHistoryLogs(ctx context.Context, householdID int64, start, end time.Time) ([]ChoreLog, bool, error) {
	return s.store.HistoryLogs(ctx, householdID, start, end)
}
