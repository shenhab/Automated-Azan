// Package notify sends desktop OS notifications on Linux, macOS, and Windows.
// All functions are best-effort: errors are returned but a failure to notify
// does not affect prayer scheduling or speaker playback.
package notify

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// Send shows a desktop notification with the given title and message.
// Uses the native notification system on each OS:
//   - Linux:   notify-send (libnotify)
//   - macOS:   osascript (always available)
//   - Windows: PowerShell toast (Windows 10+)
func Send(title, message string) error {
	switch runtime.GOOS {
	case "linux":
		return exec.Command(
			"notify-send",
			"--app-name=Automated Azan",
			"--urgency=normal",
			title, message,
		).Run()

	case "darwin":
		script := fmt.Sprintf(`display notification %q with title %q`, message, title)
		return exec.Command("osascript", "-e", script).Run()

	case "windows":
		// Windows 10+ toast via PowerShell WinRT bridge.
		// Single-quotes in the XML payload are doubled to escape them inside
		// the PowerShell single-quoted string.
		xmlBody := fmt.Sprintf(
			`<toast><visual><binding template="ToastGeneric"><text>%s</text><text>%s</text></binding></visual></toast>`,
			escapeXML(title), escapeXML(message),
		)
		ps := fmt.Sprintf(
			`[Windows.UI.Notifications.ToastNotificationManager,Windows.UI.Notifications,ContentType=WindowsRuntime]|Out-Null;`+
				`[Windows.Data.Xml.Dom.XmlDocument,Windows.Data.Xml.Dom,ContentType=WindowsRuntime]|Out-Null;`+
				`$x=New-Object Windows.Data.Xml.Dom.XmlDocument;`+
				`$x.LoadXml('%s');`+
				`[Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier('Automated Azan').Show([Windows.UI.Notifications.ToastNotification]::new($x))`,
			strings.ReplaceAll(xmlBody, "'", "''"), // escape PS single-quotes
		)
		return exec.Command(
			"powershell",
			"-NoProfile", "-NonInteractive", "-WindowStyle", "Hidden",
			"-Command", ps,
		).Run()
	}
	return nil
}

func escapeXML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, `"`, "&quot;")
	return s
}
