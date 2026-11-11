// Package notify shows desktop notifications with the tools each OS ships:
// osascript on macOS, PowerShell toasts on Windows, notify-send on Linux.
// The title and message are passed as arguments or environment variables,
// never pasted into a script, so no text can run as code.
package notify

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Run starts a program and waits for it. env adds variables to the
// program's environment.
type Run func(ctx context.Context, env []string, name string, args ...string) error

// Notifier shows a notification.
type Notifier interface {
	Notify(ctx context.Context, title, message string) error
}

// ErrUnsupported means this system has no way to show notifications.
var ErrUnsupported = errors.New("desktop notifications are not supported here")

// System returns the notifier for goos ("darwin", "windows", "linux", …).
func System(goos string, run Run) Notifier {
	return notifier{goos: goos, run: run}
}

type notifier struct {
	goos string
	run  Run
}

// appleScript reads the title and message from its arguments.
const appleScript = `on run argv
display notification (item 2 of argv) with title (item 1 of argv)
end run`

// toastScript reads the title and message from the environment. The app ID
// is PowerShell's, which Windows always allows to show toasts.
const toastScript = `$ErrorActionPreference = 'Stop'
[Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType = WindowsRuntime] > $null
$xml = [Windows.UI.Notifications.ToastNotificationManager]::GetTemplateContent([Windows.UI.Notifications.ToastTemplateType]::ToastText02)
$text = $xml.GetElementsByTagName('text')
$text.Item(0).AppendChild($xml.CreateTextNode($env:PORTCHECK_NOTIFY_TITLE)) > $null
$text.Item(1).AppendChild($xml.CreateTextNode($env:PORTCHECK_NOTIFY_MESSAGE)) > $null
$app = '{1AC14E77-02E7-4E5D-B744-2EB1AE5198B7}\WindowsPowerShell\v1.0\powershell.exe'
[Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier($app).Show([Windows.UI.Notifications.ToastNotification]::new($xml))`

func (n notifier) Notify(ctx context.Context, title, message string) error {
	var err error
	switch n.goos {
	case "darwin":
		err = n.run(ctx, nil, "osascript", "-e", appleScript, title, message)
	case "windows":
		err = n.run(ctx, []string{"PORTCHECK_NOTIFY_TITLE=" + title, "PORTCHECK_NOTIFY_MESSAGE=" + message},
			"powershell", "-NoProfile", "-NonInteractive", "-Command", toastScript)
	case "linux", "freebsd", "openbsd", "netbsd":
		err = n.run(ctx, nil, "notify-send", "--app-name=portcheck", title, message)
	default:
		return ErrUnsupported
	}
	if err != nil {
		return fmt.Errorf("show a notification: %w", err)
	}
	return nil
}

// Exec is the real Run: it starts name with the current environment plus env.
func Exec(ctx context.Context, env []string, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...) //nolint:gosec // fixed programs (osascript, powershell, notify-send); text is passed as data
	cmd.Env = append(os.Environ(), env...)
	if out, err := cmd.CombinedOutput(); err != nil {
		if msg := strings.TrimSpace(string(out)); msg != "" {
			return fmt.Errorf("%s: %w: %s", name, err, msg)
		}
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}
