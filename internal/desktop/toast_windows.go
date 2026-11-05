//go:build windows
// +build windows

package desktop

// The notification script and the defaults applied in script() are from
// https://github.com/go-toast/toast
// Copyright (c) 2016 Jacob Marshall

// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:

// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.

// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

// The script is used unchanged. Stash used to call that package's Push, which starts PowerShell with
// os/exec directly. That PowerShell shared Stash's console, so a Ctrl-C or
// Ctrl-Break sent to Stash also stopped it, and the notification failed with
// exit status 0xc000013a. Running the same script through pkg/exec gives
// PowerShell its own hidden console instead.

import (
	"bytes"
	"os"
	"text/template"

	stashExec "github.com/stashapp/stash/pkg/exec"
)

var toastTemplate = template.Must(template.New("toast").Parse(`
[Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType = WindowsRuntime] | Out-Null
[Windows.UI.Notifications.ToastNotification, Windows.UI.Notifications, ContentType = WindowsRuntime] | Out-Null
[Windows.Data.Xml.Dom.XmlDocument, Windows.Data.Xml.Dom.XmlDocument, ContentType = WindowsRuntime] | Out-Null

$APP_ID = '{{if .AppID}}{{.AppID}}{{else}}Windows App{{end}}'

$template = @"
<toast activationType="{{.ActivationType}}" launch="{{.ActivationArguments}}" duration="{{.Duration}}">
    <visual>
        <binding template="ToastGeneric">
            {{if .Icon}}
            <image placement="appLogoOverride" src="{{.Icon}}" />
            {{end}}
            {{if .Title}}
            <text><![CDATA[{{.Title}}]]></text>
            {{end}}
            {{if .Message}}
            <text><![CDATA[{{.Message}}]]></text>
            {{end}}
        </binding>
    </visual>
    {{if ne .Audio "silent"}}
	<audio src="{{.Audio}}" loop="{{.Loop}}" />
	{{else}}
	<audio silent="true" />
	{{end}}
    {{if .Actions}}
    <actions>
        {{range .Actions}}
        <action activationType="{{.Type}}" content="{{.Label}}" arguments="{{.Arguments}}" />
        {{end}}
    </actions>
    {{end}}
</toast>
"@

$xml = New-Object Windows.Data.Xml.Dom.XmlDocument
$xml.LoadXml($template)
$toast = New-Object Windows.UI.Notifications.ToastNotification $xml
[Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier($APP_ID).Show($toast)
    `))

type toastAction struct {
	Type      string
	Label     string
	Arguments string
}

type toastNotification struct {
	AppID               string
	Title               string
	Message             string
	Icon                string
	ActivationType      string
	ActivationArguments string
	Actions             []toastAction
	Audio               string
	Loop                bool
	Duration            string
}

// script returns the PowerShell script that shows the notification, with the
// same defaults go-toast applied.
func (n toastNotification) script() (string, error) {
	if n.ActivationType == "" {
		n.ActivationType = "protocol"
	}
	if n.Duration == "" {
		n.Duration = "short"
	}
	if n.Audio == "" {
		n.Audio = "ms-winsoundevent:Notification.Default"
	}
	var out bytes.Buffer
	if err := toastTemplate.Execute(&out, n); err != nil {
		return "", err
	}
	return out.String(), nil
}

// push shows the notification by running its script in PowerShell.
func (n toastNotification) push() error {
	content, err := n.script()
	if err != nil {
		return err
	}

	f, err := os.CreateTemp("", "stash-toast-*.ps1")
	if err != nil {
		return err
	}
	file := f.Name()
	defer os.Remove(file)

	// PowerShell reads the script as UTF-8 only when it starts with a BOM.
	bomUtf8 := []byte{0xEF, 0xBB, 0xBF}
	_, err = f.Write(append(bomUtf8, []byte(content)...))
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}

	return stashExec.Command("PowerShell", "-ExecutionPolicy", "Bypass", "-File", file).Run()
}
