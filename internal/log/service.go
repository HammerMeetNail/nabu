package log

import (
	"context"
	"errors"
	"time"
)

var ErrNotFound = errors.New("log entry not found")

type Service struct {
	store Store
	now   func() time.Time
}

func NewService(store Store) *Service {
	return &Service{
		store: store,
		now:   func() time.Time { return time.Now().UTC() },
	}
}

func (s *Service) LogChore(ctx context.Context, householdID, userID, choreID int64, note string, indicators []string, indicatorVolumes map[string]int, date *time.Time, slotHour *int, completedAt *time.Time, volumeML *int) (ChoreLog, error) {
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
	return s.store.CreateLog(ctx, ChoreLog{
		HouseholdID:      householdID,
		UserID:           userID,
		ChoreID:          choreID,
		CompletedAt:      logCompletedAt,
		Note:             note,
		Indicators:       indicators,
		IndicatorVolumes: indicatorVolumes,
		SlotHour:         slotHour,
		LogDate:          logDate,
		VolumeML:         volumeML,
	})
}

func (s *Service) UpdateLog(ctx context.Context, logID int64, householdID int64, note string, indicators []string, indicatorVolumes map[string]int, volumeML *int, userID *int64, completedAt *time.Time, slotHour *int, logDate *time.Time) error {
	log, err := s.store.GetLog(ctx, logID)
	if err != nil {
		return err
	}
	if log.HouseholdID != householdID {
		return errors.New("log does not belong to your household")
	}
	log.Note = note
	if indicators == nil {
		indicators = []string{}
	}
	log.Indicators = indicators
	log.IndicatorVolumes = indicatorVolumes
	log.VolumeML = volumeML
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
	return s.store.UpdateLog(ctx, log)
}

func (s *Service) UndoLog(ctx context.Context, householdID, logID int64) error {
	log, err := s.store.GetLog(ctx, logID)
	if err != nil {
		return err
	}
	if log.HouseholdID != householdID {
		return errors.New("can only undo logs in your own household")
	}
	return s.store.DeleteLog(ctx, logID)
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
