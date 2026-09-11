package notification

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"time"
)

const PageSize = 50

var ErrInvalidCursor = errors.New("invalid notification cursor")

// Cursor is a position, never an authorization capability. Every store query
// independently restricts results to the authenticated recipient.
type Cursor struct {
	At time.Time `json:"at"`
	ID int64     `json:"id"`
}
type Page struct {
	Notifications []Notification `json:"notifications"`
	UnreadCount   int            `json:"unreadCount"`
	NextCursor    string         `json:"nextCursor,omitempty"`
}

func DecodeCursor(raw string) (*Cursor, error) {
	if raw == "" {
		return nil, nil
	}
	if len(raw) > 256 {
		return nil, ErrInvalidCursor
	}
	data, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return nil, ErrInvalidCursor
	}
	var c Cursor
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err = dec.Decode(&c); err != nil || c.ID <= 0 || c.At.IsZero() || c.At.Year() < 1 || c.At.Year() > 9999 {
		return nil, ErrInvalidCursor
	}
	var extra any
	if dec.Decode(&extra) != io.EOF {
		return nil, ErrInvalidCursor
	}
	return &c, nil
}
func encodeCursor(n Notification) string {
	data, _ := json.Marshal(Cursor{At: n.CreatedAt, ID: n.ID})
	return base64.RawURLEncoding.EncodeToString(data)
}
func (s *Service) ListPage(ctx context.Context, userID int64, raw string) (Page, error) {
	before, err := DecodeCursor(raw)
	if err != nil {
		return Page{}, err
	}
	rows, err := s.store.ListNotificationsBefore(ctx, userID, before, PageSize+1)
	if err != nil {
		return Page{}, err
	}
	page := Page{Notifications: rows}
	if len(rows) > PageSize {
		page.Notifications = rows[:PageSize]
		page.NextCursor = encodeCursor(rows[PageSize-1])
	}
	if page.Notifications == nil {
		page.Notifications = []Notification{}
	}
	page.UnreadCount, err = s.store.GetUnreadCount(ctx, userID)
	return page, err
}
