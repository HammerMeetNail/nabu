package app

import (
	"net/http"
	"strings"
)

// Static legal/support pages required for App Store submission (P6/A5):
// App Store Connect needs a privacy-policy URL and a support URL, and the
// iOS app's Settings → About links here. Served ahead of the SPA catch-all.

const pageShell = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>%TITLE% — Nabu</title>
<style>
body { font-family: -apple-system, system-ui, sans-serif; max-width: 42rem; margin: 0 auto; padding: 2rem 1.25rem; line-height: 1.6; color: #1c2b33; background: #f4efe7; }
h1 { font-size: 1.5rem; } h2 { font-size: 1.15rem; margin-top: 1.75rem; }
a { color: #236886; }
@media (prefers-color-scheme: dark) { body { color: #e8e2d6; background: #0e1117; } a { color: #4dabce; } }
</style>
</head>
<body>
%BODY%
<p><a href="/">← Back to Nabu</a></p>
</body>
</html>`

const privacyBody = `
<h1>Privacy Policy</h1>
<p><em>Last updated: 2026-10-29</em></p>
<p>Nabu is a household activity tracker. This policy describes what data the
service stores and how it is used.</p>

<h2>Data we collect</h2>
<ul>
<li><strong>Account data:</strong> your email address and, optionally, a
display name. Used to sign you in and to identify you to other members of
your household.</li>
<li><strong>Household activity data:</strong> the chores, logs, schedules,
and notes you and your household members create. This is the product's
content and is visible to the members of your household.</li>
<li><strong>Push tokens:</strong> if you enable notifications, a device
push token used only to deliver your reminders.</li>
</ul>

<h2>What we don't do</h2>
<ul>
<li>We don't run advertising, third-party analytics, or tracking SDKs.</li>
<li>We don't sell or share your household's data with third parties.</li>
<li>We don't use your data to train machine-learning models.</li>
<li>If any of these practices change, we will update this policy before the
change takes effect and let you know first.</li>
</ul>

<h2>Getting a copy of your data</h2>
<p>You can download your data at any time from Settings → Account →
Download My Data (in the app or on the web). It is one JSON file with your
account, households, preferences, notification and reminder settings, the
logs you recorded in every household, the day notes you last edited, your
notifications, where you are signed in, your devices and the names of your
API keys. Sign-in tokens, push endpoints and device tokens are left out,
because they work as credentials. The logs you recorded are also available
as a CSV file.</p>

<h2>Data retention and deletion</h2>
<p>You can delete your account at any time from Settings → Account →
Delete Account (in the app or on the web). Before deleting, you can download
your data as described above. Deletion is immediate and permanent.</p>
<p><strong>What is deleted:</strong> your account, email address, display
name and avatar, sessions, push and device tokens, preferences, reminder
settings, notifications, sign-in and email tokens, and invites you created.
Households where you are the only member are deleted with all their data.</p>
<p><strong>What is kept:</strong> in households you share with others, the
logs you recorded stay so the other members keep their household's history
(for example, a child's feeding record). They are shown as "Former member"
and are no longer linked to your account, email or name. When deleting, you
can choose to also delete everything you logged, or only the notes you wrote
on those logs. Chores and schedules you created in a shared household stay
with that household, without your name.</p>

<h2>Contact</h2>
<p>Questions about this policy: see the <a href="/support">support page</a>.</p>
`

const supportBody = `
<h1>Support</h1>
<p>Nabu is a household activity tracker for logging feeds, chores, and
routines together.</p>
<h2>Get help</h2>
<ul>
<li>Report issues or ask questions on
<a href="https://github.com/HammerMeetNail/nabu/issues">GitHub Issues</a>.</li>
</ul>
<h2>Common tasks</h2>
<ul>
<li><strong>Reset your password:</strong> use "Forgot password?" on the sign-in screen.</li>
<li><strong>Join a household:</strong> open the invite link a member shared with you.</li>
<li><strong>Delete your account:</strong> Settings → Account → Delete Account.</li>
</ul>
`

func registerStaticPages(mux *http.ServeMux) {
	serve := func(title, body string) http.HandlerFunc {
		html := strings.NewReplacer("%TITLE%", title, "%BODY%", body).Replace(pageShell)
		page := []byte(html)
		return func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Cache-Control", "no-store")
			w.Write(page) //nolint:errcheck
		}
	}
	mux.HandleFunc("/privacy", method(http.MethodGet, serve("Privacy Policy", privacyBody)))
	mux.HandleFunc("/support", method(http.MethodGet, serve("Support", supportBody)))
}
