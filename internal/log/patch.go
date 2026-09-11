package log

// LogFields contains JSON field names explicitly present in a PATCH. A nil
// mask preserves the full-update contract for internal callers; an empty mask
// changes nothing. Identity fields are never writable.
type LogFields map[string]bool

type Patch struct {
	ActorID int64
	Fields  LogFields
}

func (f LogFields) includes(name string) bool { return f == nil || f[name] }
func fieldsOrAll(fields []LogFields) LogFields {
	if len(fields) > 0 {
		return fields[0]
	}
	return nil
}
func mergeLog(existing, next ChoreLog, fields LogFields) ChoreLog {
	if fields.includes("note") {
		existing.Note = next.Note
	}
	if fields.includes("title") {
		existing.Title = next.Title
	}
	if fields.includes("indicators") {
		existing.Indicators = nilToEmptyLog(next.Indicators)
	}
	if fields.includes("indicatorVolumes") {
		existing.IndicatorVolumes = next.IndicatorVolumes
	}
	if fields.includes("volumeML") {
		existing.VolumeML = next.VolumeML
	}
	if fields.includes("rating") {
		existing.Rating = next.Rating
	}
	if fields.includes("durationSeconds") {
		existing.DurationSeconds = next.DurationSeconds
	}
	if fields.includes("subject") {
		existing.Subject = next.Subject
	}
	if fields.includes("userId") {
		existing.UserID = next.UserID
	}
	if fields.includes("completedAt") {
		existing.CompletedAt = next.CompletedAt
	}
	if fields.includes("hour") {
		existing.SlotHour = next.SlotHour
	}
	if fields.includes("date") {
		existing.LogDate = next.LogDate
	}
	return existing
}
