package log

import (
	"context"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

func validateUnit(unit string) error {
	if strings.TrimSpace(unit) == "" || utf8.RuneCountInString(unit) > 12 || strings.IndexFunc(unit, unicode.IsControl) >= 0 {
		return fmt.Errorf("%w: unit must contain 1 to 12 characters without control characters", ErrInvalidInput)
	}
	return nil
}

// AmountUnit groups liquid storage (always canonical mL) without combining
// unrelated custom units such as mg and g.
func AmountUnit(unit string) string {
	switch strings.ToLower(strings.TrimSpace(unit)) {
	case "", "ml", "oz":
		return "mL"
	}
	return unit
}

func (s *Service) snapshotUnit(ctx context.Context, actorID, householdID, choreID int64, requested string) (string, error) {
	if s.chores == nil {
		return requested, nil
	}
	c, err := s.chores.GetVisible(ctx, actorID, householdID, choreID)
	if err != nil {
		return "", ErrNotFound
	}
	if requested != "" {
		if !c.HasVolumeML {
			return "", fmt.Errorf("%w: this activity does not track an amount", ErrInvalidInput)
		}
		return requested, nil
	}
	if c.HasVolumeML {
		if c.MetricUnit != "" {
			return c.MetricUnit, nil
		}
		return "mL", nil
	}
	return "", nil
}
