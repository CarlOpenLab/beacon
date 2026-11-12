package main

// The 通知 page is where the switches for the three events live, and the one
// control that answers "will a notification actually appear on this machine?":
// the machine this window runs on is the only place that question can be
// answered, and the answer is otherwise invisible until something goes wrong.

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/beacon/locales"
)

// openNotificationsPane opens the settings window on the 通知 page.
func openNotificationsPane(t *testing.T, sw *settingsWindow) *ui.Tester {
	t.Helper()
	tt := ui.NewTester(sw.view, 640, 480)
	tt.Frame()
	if err := tt.Click(locales.SettingsPaneNotifications()); err != nil {
		t.Fatalf("Click(%s): %v", locales.SettingsPaneNotifications(), err)
	}
	tt.Frame()
	return tt
}

// settleCanNotify waits for the window's own question about this machine to be
// answered. The window asks it when it opens, off the UI thread, because the
// answer comes back over the app's event loop; the page draws what arrived.
func settleCanNotify(t *testing.T, sw *settingsWindow, want bool) {
	t.Helper()
	for range 200 {
		sw.desk.drain()
		if sw.canNotify == want {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("canNotify = %v, want %v", sw.canNotify, want)
}

// stubNotifications replaces the two calls the page makes into the system's
// notifications with fakes that record what the page asked for, the way
// updates.go's does for the updater plugin: mygo needs the running app's event
// loop to answer either, which a test process does not have.
type notificationStub struct {
	supported bool
	posted    []string
	err       error
}

func stubNotifications(t *testing.T, supported bool) *notificationStub {
	t.Helper()
	st := &notificationStub{supported: supported}
	ask, post := notificationsSupported, postTestNotification
	t.Cleanup(func() { notificationsSupported, postTestNotification = ask, post })

	notificationsSupported = func() bool { return st.supported }
	postTestNotification = func(title, body string) error {
		st.posted = append(st.posted, title+"｜"+body)
		return st.err
	}
	return st
}

// settleNotifications waits for the click's notification to land. The post runs
// off the UI thread — Show returns once the system has it, which on macOS can be
// the wait for the person answering — so the footer's answer arrives a moment
// after the frame of the click, through the window's own queue.
func settleNotifications(t *testing.T, sw *settingsWindow) {
	t.Helper()
	for range 100 {
		sw.desk.drain()
		if sw.status != "" {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("the page never answered the click: status = %q", sw.status)
}

// Sending a test notification posts one carrying the page's own words, and says
// so in the footer — the point being that the user sees a banner without having
// to wait for a session to need them.
func TestTheNotificationsPagePostsATestNotification(t *testing.T) {
	stub := stubNotifications(t, true)
	sw, _ := newTestSettingsWindow(t)
	settleCanNotify(t, sw, true)

	tt := openNotificationsPane(t, sw)
	if _, ok := tt.Find(locales.SettingsNotifySend()); !ok {
		t.Fatalf("the 通知 page has no test button: %q", tt.Texts())
	}
	if err := tt.Click(locales.SettingsNotifySend()); err != nil {
		t.Fatalf("Click(%s): %v", locales.SettingsNotifySend(), err)
	}
	tt.Frame()
	settleNotifications(t, sw)

	want := locales.NotifyTestTitle() + "｜" + locales.NotifyTestBody()
	if len(stub.posted) != 1 || stub.posted[0] != want {
		t.Fatalf("posted %q, want one %q", stub.posted, want)
	}
	if sw.status != locales.SettingsNotifyTested() || sw.statusErr {
		t.Fatalf("status = %q (err %v), want %q", sw.status, sw.statusErr, locales.SettingsNotifyTested())
	}
}

// A machine that cannot post notifications — a browser, or a build that is not
// a bundle — has no button to press, and a sentence saying why instead of one
// that would do nothing.
func TestTheNotificationsPageOffersNoTestWhereNoneCanAppear(t *testing.T) {
	stub := stubNotifications(t, false)
	sw, _ := newTestSettingsWindow(t)
	settleCanNotify(t, sw, false)

	tt := openNotificationsPane(t, sw)
	if _, ok := tt.Find(locales.SettingsNotifySend()); ok {
		t.Error("a machine that cannot post notifications should not offer the test button")
	}
	if !tt.HasText(locales.SettingsNotifyOffNote()) {
		t.Fatalf("the page does not say why there is no test: %q", tt.Texts())
	}
	if len(stub.posted) != 0 {
		t.Errorf("posted %q on a machine with no notifications", stub.posted)
	}
}

// The system refusing the notification — macOS asking and the person saying no
// — is the answer the page was for, so it is reported rather than swallowed.
func TestTheNotificationsPageReportsARefusedNotification(t *testing.T) {
	stub := stubNotifications(t, true)
	stub.err = mygo.ErrNotificationsDenied
	sw, _ := newTestSettingsWindow(t)
	settleCanNotify(t, sw, true)

	tt := openNotificationsPane(t, sw)
	if err := tt.Click(locales.SettingsNotifySend()); err != nil {
		t.Fatalf("Click(%s): %v", locales.SettingsNotifySend(), err)
	}
	tt.Frame()
	settleNotifications(t, sw)

	if !sw.statusErr || !strings.Contains(sw.status, locales.SettingsNotifyDenied()) {
		t.Fatalf("status = %q (err %v), want the refusal", sw.status, sw.statusErr)
	}
}

// An error that is not the refusal — a machine whose notification service is
// broken — is reported as itself, with its reason, since it is the one the page
// exists to surface.
func TestTheNotificationsPageReportsAFailedNotification(t *testing.T) {
	stub := stubNotifications(t, true)
	stub.err = errors.New("no D-Bus")
	sw, _ := newTestSettingsWindow(t)
	settleCanNotify(t, sw, true)

	tt := openNotificationsPane(t, sw)
	if err := tt.Click(locales.SettingsNotifySend()); err != nil {
		t.Fatalf("Click(%s): %v", locales.SettingsNotifySend(), err)
	}
	tt.Frame()
	settleNotifications(t, sw)

	if !sw.statusErr || !strings.Contains(sw.status, "no D-Bus") {
		t.Fatalf("status = %q (err %v), want the reason", sw.status, sw.statusErr)
	}
}
