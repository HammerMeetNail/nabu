package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"time"

	"github.com/HammerMeetNail/nabu/internal/apikey"
	"github.com/HammerMeetNail/nabu/internal/apns"
	"github.com/HammerMeetNail/nabu/internal/auth"
	"github.com/HammerMeetNail/nabu/internal/chore"
	"github.com/HammerMeetNail/nabu/internal/daynote"
	"github.com/HammerMeetNail/nabu/internal/household"
	logsvc "github.com/HammerMeetNail/nabu/internal/log"
	"github.com/HammerMeetNail/nabu/internal/middleware"
	"github.com/HammerMeetNail/nabu/internal/notification"
	"github.com/HammerMeetNail/nabu/internal/push"
	"github.com/HammerMeetNail/nabu/internal/reminder"
	"github.com/HammerMeetNail/nabu/internal/userprefs"
)

// MyDataSources are the stores and services a personal data export reads.
type MyDataSources struct {
	Sessions interface {
		ListUserSessions(ctx context.Context, userID int64, currentHash string) ([]auth.SessionInfo, error)
	}
	Households  household.Store
	Chores      chore.Store
	Preferences interface {
		GetPreferences(ctx context.Context, userID int64) (userprefs.Preferences, error)
	}
	Notifications interface {
		GetNotificationPreferences(ctx context.Context, userID int64) (notification.ReminderPreference, error)
		ListPage(ctx context.Context, userID int64, raw string) (notification.Page, error)
	}
	Reminders interface {
		GetChoreReminderPrefs(ctx context.Context, userID int64) ([]reminder.ChoreReminderPref, error)
	}
	Logs interface {
		GetUserLogs(ctx context.Context, userID int64) ([]logsvc.ChoreLog, error)
	}
	DayNotes interface {
		ListRange(ctx context.Context, householdID int64, start, end time.Time) ([]daynote.DayNote, error)
	}
	WebPush interface {
		GetSubscriptions(ctx context.Context, userID int64) ([]push.Subscription, error)
	}
	Devices interface {
		DevicesForUser(ctx context.Context, userID int64) ([]apns.Device, error)
	}
	APIKeys interface {
		List(ctx context.Context, user auth.User) ([]apikey.Key, error)
	}
}

// MyDataHandler serves GET /api/me/export: everything Nabu keeps about the
// signed-in user, as one JSON file (GDPR articles 15 and 20). It leaves out
// every token, hash and push endpoint, which act as credentials.
type MyDataHandler struct{ src MyDataSources }

func NewMyDataHandler(src MyDataSources) *MyDataHandler { return &MyDataHandler{src: src} }

// The newest notifications go in the file; older ones are only flagged.
const myDataNotificationLimit = 1000

const myDataAbout = "Everything Nabu keeps about your account. Logs are the ones recorded for you, in every household, " +
	"including those you have left; activity and household names appear where you can still see them. Day notes are " +
	"shared by a household, so only notes you last edited are listed. Ratings are in tenths of a star. Sign-in tokens, " +
	"push endpoints and device tokens are left out because they work as credentials."

type myDataExport struct {
	ExportedAt              time.Time                       `json:"exportedAt"`
	About                   string                          `json:"about"`
	Account                 myDataAccount                   `json:"account"`
	Households              []myDataHousehold               `json:"households"`
	Preferences             userprefs.Preferences           `json:"preferences"`
	NotificationPreferences notification.ReminderPreference `json:"notificationPreferences"`
	ReminderPreferences     []reminder.ChoreReminderPref    `json:"reminderPreferences"`
	Logs                    []myDataLog                     `json:"logs"`
	DayNotes                []myDataDayNote                 `json:"dayNotes"`
	Notifications           []notification.Notification     `json:"notifications"`
	NotificationsTruncated  bool                            `json:"notificationsTruncated,omitempty"`
	SignIns                 []auth.SessionInfo              `json:"signIns"`
	Devices                 []myDataDevice                  `json:"devices"`
	APIKeys                 []apikey.Key                    `json:"apiKeys"`
}

type myDataAccount struct {
	ID            int64     `json:"id"`
	Email         string    `json:"email"`
	DisplayName   string    `json:"displayName"`
	AvatarColor   string    `json:"avatarColor"`
	EmailVerified bool      `json:"emailVerified"`
	HasPassword   bool      `json:"hasPassword"`
	CreatedAt     time.Time `json:"createdAt"`
}

type myDataHousehold struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Role      string `json:"role"`
	Suspended bool   `json:"suspended"`
}

type myDataLog struct {
	logsvc.ChoreLog
	Household string `json:"household,omitempty"`
	Activity  string `json:"activity,omitempty"`
}

type myDataDayNote struct {
	HouseholdID int64     `json:"householdId"`
	Household   string    `json:"household"`
	Date        string    `json:"date"`
	Note        string    `json:"note"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type myDataDevice struct {
	Kind        string `json:"kind"`
	Name        string `json:"name,omitempty"`
	Service     string `json:"service,omitempty"`
	Environment string `json:"environment,omitempty"`
}

func (h *MyDataHandler) Export(w http.ResponseWriter, r *http.Request) {
	r, cancel := personalExportContext(r)
	defer cancel()
	user, ok := middleware.CurrentUser(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	doc, err := h.collect(r.Context(), user)
	if err != nil {
		personalExportError(w, err)
		return
	}
	buffer := &exportBuffer{ctx: r.Context()}
	encoder := json.NewEncoder(buffer)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(doc); err != nil {
		personalExportError(w, err)
		return
	}
	sendFile(w, r, buffer, "nabu-my-data.json", "application/json; charset=utf-8", personalExportError)
}

func (h *MyDataHandler) collect(ctx context.Context, user auth.User) (myDataExport, error) {
	doc := myDataExport{
		ExportedAt: time.Now().UTC(),
		About:      myDataAbout,
		Account: myDataAccount{ID: user.ID, Email: user.Email, DisplayName: user.DisplayName, AvatarColor: user.AvatarColor,
			EmailVerified: user.EmailVerified, HasPassword: user.HasPassword, CreatedAt: user.CreatedAt},
		Households: []myDataHousehold{}, ReminderPreferences: []reminder.ChoreReminderPref{}, Logs: []myDataLog{},
		DayNotes: []myDataDayNote{}, Notifications: []notification.Notification{}, SignIns: []auth.SessionInfo{},
		Devices: []myDataDevice{}, APIKeys: []apikey.Key{},
	}

	current, err := h.src.Households.ListUserHouseholds(ctx, user.ID)
	if err != nil {
		return doc, err
	}
	suspended, err := h.src.Households.GetSuspendedMemberships(ctx, user.ID)
	if err != nil {
		return doc, err
	}
	householdNames := map[int64]string{}
	activityNames := map[int64]string{}
	chores := chore.NewService(h.src.Chores).WithMemberships(h.src.Households)
	for _, hh := range current {
		doc.Households = append(doc.Households, myDataHousehold{ID: hh.ID, Name: hh.Name, Role: hh.Role})
		householdNames[hh.ID] = hh.Name
		visible, err := chores.ListVisible(ctx, user.ID, hh.ID)
		if err != nil {
			return doc, err
		}
		for _, c := range visible {
			activityNames[c.ID] = c.Name
		}
		notes, err := h.src.DayNotes.ListRange(ctx, hh.ID, householdExportStart, householdExportEnd)
		if err != nil {
			return doc, err
		}
		for _, note := range notes {
			if note.UpdatedBy != nil && *note.UpdatedBy == user.ID {
				doc.DayNotes = append(doc.DayNotes, myDataDayNote{HouseholdID: hh.ID, Household: hh.Name,
					Date: note.Date, Note: note.Note, UpdatedAt: note.UpdatedAt})
			}
		}
	}
	for _, hh := range suspended {
		doc.Households = append(doc.Households, myDataHousehold{ID: hh.ID, Name: hh.Name, Role: hh.Role, Suspended: true})
	}

	if doc.Preferences, err = h.src.Preferences.GetPreferences(ctx, user.ID); err != nil {
		return doc, err
	}
	if doc.NotificationPreferences, err = h.src.Notifications.GetNotificationPreferences(ctx, user.ID); err != nil {
		return doc, err
	}
	prefs, err := h.src.Reminders.GetChoreReminderPrefs(ctx, user.ID)
	if err != nil {
		return doc, err
	}
	doc.ReminderPreferences = append(doc.ReminderPreferences, prefs...)

	logs, err := h.src.Logs.GetUserLogs(ctx, user.ID)
	if err != nil {
		return doc, err
	}
	for _, l := range logs {
		doc.Logs = append(doc.Logs, myDataLog{ChoreLog: l, Household: householdNames[l.HouseholdID], Activity: activityNames[l.ChoreID]})
	}

	for cursor := ""; ; {
		page, err := h.src.Notifications.ListPage(ctx, user.ID, cursor)
		if err != nil {
			return doc, err
		}
		for _, n := range page.Notifications {
			if len(doc.Notifications) == myDataNotificationLimit {
				doc.NotificationsTruncated = true
				break
			}
			doc.Notifications = append(doc.Notifications, n)
		}
		if doc.NotificationsTruncated || page.NextCursor == "" {
			break
		}
		cursor = page.NextCursor
	}

	sessions, err := h.src.Sessions.ListUserSessions(ctx, user.ID, user.SessionHash)
	if err != nil {
		return doc, err
	}
	doc.SignIns = append(doc.SignIns, sessions...)

	subscriptions, err := h.src.WebPush.GetSubscriptions(ctx, user.ID)
	if err != nil {
		return doc, err
	}
	for _, sub := range subscriptions {
		// The endpoint's path is a capability; its host only names the service.
		service := ""
		if u, err := url.Parse(sub.Endpoint); err == nil {
			service = u.Hostname()
		}
		doc.Devices = append(doc.Devices, myDataDevice{Kind: "web push", Service: service})
	}
	devices, err := h.src.Devices.DevicesForUser(ctx, user.ID)
	if err != nil {
		return doc, err
	}
	for _, d := range devices {
		doc.Devices = append(doc.Devices, myDataDevice{Kind: "iPhone", Name: d.DeviceName, Environment: string(d.Environment)})
	}

	keys, err := h.src.APIKeys.List(ctx, user)
	if err != nil {
		return doc, err
	}
	doc.APIKeys = append(doc.APIKeys, keys...)
	return doc, nil
}
