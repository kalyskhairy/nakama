package main

import (
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"os/exec"
	"unicode/utf16"

	"github.com/kalyskhairy/nakama/internal/app"
)

// toastScript is the PowerShell that shows a Windows notification. The texts come from session
// titles, so they go in as base64 and are never part of the code.
func toastScript(n app.Notice) string {
	b64 := func(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }
	return fmt.Sprintf(`$null = [Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType = WindowsRuntime]
$x = [Windows.UI.Notifications.ToastNotificationManager]::GetTemplateContent([Windows.UI.Notifications.ToastTemplateType]::ToastText02)
$t = $x.GetElementsByTagName('text')
$d = { param($s) [Text.Encoding]::UTF8.GetString([Convert]::FromBase64String($s)) }
$null = $t.Item(0).AppendChild($x.CreateTextNode((& $d '%s')))
$null = $t.Item(1).AppendChild($x.CreateTextNode((& $d '%s')))
$app = '{1AC14E77-02E7-4E5D-B744-2EB1AE5198B7}\WindowsPowerShell\v1.0\powershell.exe'
[Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier($app).Show([Windows.UI.Notifications.ToastNotification]::new($x))
`, b64(n.Title), b64(n.Text))
}

// toast shows the notification without waiting for it; outside WSL it does nothing.
func toast(n app.Notice) {
	path, err := exec.LookPath("powershell.exe")
	if err != nil {
		return
	}
	var u []byte
	for _, r := range utf16.Encode([]rune(toastScript(n))) {
		u = binary.LittleEndian.AppendUint16(u, r)
	}
	c := exec.Command(path, "-NoProfile", "-NonInteractive", "-EncodedCommand", base64.StdEncoding.EncodeToString(u))
	if c.Start() == nil {
		go c.Wait()
	}
}
