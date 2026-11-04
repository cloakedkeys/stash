//go:build windows

package desktop

import (
	"strings"
	"testing"
)

// The script must stay the one go-toast's Push produced for the same
// notification, defaults included.
func TestToastScript(t *testing.T) {
	n := toastNotification{
		AppID:   "Stash",
		Title:   "Stash has moved!",
		Message: "Stash now runs in your tray, instead of a terminal window.",
		Icon:    `C:\stash\icon.png`,
		Actions: []toastAction{{
			Type:      "protocol",
			Label:     "Open Stash",
			Arguments: "http://localhost:9999/",
		}},
	}

	got, err := n.script()
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{
		"$APP_ID = 'Stash'",
		`<toast activationType="protocol" launch="" duration="short">`,
		`<image placement="appLogoOverride" src="C:\stash\icon.png" />`,
		"<text><![CDATA[Stash has moved!]]></text>",
		"<text><![CDATA[Stash now runs in your tray, instead of a terminal window.]]></text>",
		`<audio src="ms-winsoundevent:Notification.Default" loop="false" />`,
		`<action activationType="protocol" content="Open Stash" arguments="http://localhost:9999/" />`,
		"[Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier($APP_ID).Show($toast)",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("script is missing %q\n%s", want, got)
		}
	}
}

func TestToastScriptDefaults(t *testing.T) {
	got, err := toastNotification{}.script()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"$APP_ID = 'Windows App'",
		`duration="short"`,
		`<audio src="ms-winsoundevent:Notification.Default" loop="false" />`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("script is missing %q", want)
		}
	}
	for _, unwanted := range []string{"<image", "<text>", "<actions>"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("empty notification should not contain %q", unwanted)
		}
	}
}
