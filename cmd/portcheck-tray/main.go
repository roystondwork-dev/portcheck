// Command portcheck-tray shows the ports you're listening on in the macOS
// menu bar or the Windows/Linux system tray: who holds each one, and stop,
// open or copy from the menu. The logic is in internal/tray; this file only
// wires it to the OS.
package main

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/binary"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"fyne.io/systray"

	"github.com/roystondwork-dev/portcheck/internal/buildinfo"
	"github.com/roystondwork-dev/portcheck/internal/notify"
	"github.com/roystondwork-dev/portcheck/internal/tray"
	"github.com/roystondwork-dev/portcheck/pkg/containers"
	"github.com/roystondwork-dev/portcheck/pkg/inspect"
	"github.com/roystondwork-dev/portcheck/pkg/killer"
	"github.com/roystondwork-dev/portcheck/pkg/monitor"
	"github.com/roystondwork-dev/portcheck/pkg/wsl"
)

var (
	//go:embed icons/template.png
	templateIcon []byte
	//go:embed icons/color.png
	colorIcon []byte
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	systray.Run(func() { onReady(ctx) }, cancel)
}

func onReady(ctx context.Context) {
	switch runtime.GOOS {
	case "darwin":
		systray.SetTemplateIcon(templateIcon, colorIcon) // tinted for light and dark menu bars
	case "windows":
		systray.SetIcon(icoFromPNG(colorIcon))
	default:
		systray.SetIcon(colorIcon)
	}
	var opts []inspect.Option
	if engine, ok := containers.Discover(os.Getenv); ok {
		opts = append(opts, inspect.WithEnricher(engine))
	}
	if runtime.GOOS == "windows" {
		opts = append(opts, inspect.WithEnricher(wsl.New()))
	}
	lister := inspect.NewSystem(opts...)
	k := killer.New()
	// Uptime monitors from `portcheck monitor add`, checked in the background.
	runner := &monitor.Runner{
		Check:   monitor.NewChecker("portcheck-tray/" + buildinfo.Get().Version).Check,
		Tracker: &monitor.Tracker{},
	}
	var sup *monitor.Supervisor
	if path := monitorsFile(); path != "" {
		sup = &monitor.Supervisor{Path: path, Runner: runner}
	}
	t := tray.New(tray.Deps{
		List: func(ctx context.Context) ([]inspect.Socket, error) {
			res, err := lister.List(ctx, inspect.Filter{ListeningOnly: true})
			if err != nil {
				return nil, err
			}
			return res.Sockets, nil
		},
		Kill: func(ctx context.Context, pid int32) (killer.Result, error) {
			return k.Kill(ctx, pid, killer.Options{})
		},
		Open:  func(url string) error { return openURL(ctx, url) },
		Copy:  func(text string) error { return copyText(ctx, text) },
		User:  currentUser(),
		Every: 3 * time.Second,
		Now:   time.Now,
		Monitors: func() []tray.MonitorView {
			if sup == nil {
				return nil
			}
			return tray.MonitorViews(sup.Targets(), runner.Tracker.States())
		},
	}, menu{})
	if sup != nil {
		n := notify.System(runtime.GOOS, notify.Exec)
		runner.OnEvent = func(e monitor.Event) {
			_ = n.Notify(ctx, "portcheck", e.Message())
			t.Refresh(ctx)
		}
		go sup.Run(ctx)
	}
	go t.Run(ctx)
}

// monitorsFile is the monitors file `portcheck monitor` uses: in
// $PORTCHECK_CONFIG_DIR, or the portcheck folder of the user's configuration
// directory. Empty if there's no such directory.
func monitorsFile() string {
	dir := os.Getenv("PORTCHECK_CONFIG_DIR")
	if dir == "" {
		base, err := os.UserConfigDir()
		if err != nil {
			return ""
		}
		dir = filepath.Join(base, "portcheck")
	}
	return filepath.Join(dir, monitor.FileName)
}

// currentUser is the name inspect reports as a process's user.
func currentUser() string {
	u, err := user.Current()
	if err != nil {
		return ""
	}
	return u.Username
}

// openURL opens a URL the tray built: http://localhost:<port>/.
func openURL(ctx context.Context, url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.CommandContext(ctx, "open", url) //nolint:gosec // url is http://localhost:<port>/
	case "windows":
		cmd = exec.CommandContext(ctx, "rundll32", "url.dll,FileProtocolHandler", url) //nolint:gosec // as above
	default:
		cmd = exec.CommandContext(ctx, "xdg-open", url) //nolint:gosec // as above
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("open %s: %w", url, err)
	}
	return nil
}

func copyText(ctx context.Context, text string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.CommandContext(ctx, "pbcopy")
	case "windows":
		cmd = exec.CommandContext(ctx, "clip")
	default:
		if _, err := exec.LookPath("wl-copy"); err == nil {
			cmd = exec.CommandContext(ctx, "wl-copy")
		} else {
			cmd = exec.CommandContext(ctx, "xclip", "-selection", "clipboard")
		}
	}
	cmd.Stdin = strings.NewReader(text)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("copy to the clipboard: %w", err)
	}
	return nil
}

// icoFromPNG wraps a PNG in an ICO container, which Windows (Vista and later)
// accepts as a tray icon.
func icoFromPNG(png []byte) []byte {
	var b bytes.Buffer
	le := func(v any) { _ = binary.Write(&b, binary.LittleEndian, v) }
	le(uint16(0))         // reserved
	le(uint16(1))         // type: icon
	le(uint16(1))         // one image
	b.Write([]byte{0, 0}) // width, height 0 = 256 or read from the PNG
	le(uint16(0))         // colour count, reserved
	le(uint16(1))         // planes
	le(uint16(32))        // bits per pixel
	le(uint32(len(png)))  // image size
	le(uint32(6 + 16))    // image offset: after the header and one entry
	b.Write(png)
	return b.Bytes()
}

// menu adapts fyne.io/systray to tray.Menu.
type menu struct{}

func (menu) Reset()     { systray.ResetMenu() }
func (menu) Separator() { systray.AddSeparator() }
func (menu) Quit()      { systray.Quit() }
func (menu) Add(title, tooltip string) tray.Item {
	return item{systray.AddMenuItem(title, tooltip)}
}
func (menu) AddCheckbox(title string, checked bool) tray.Item {
	return item{systray.AddMenuItemCheckbox(title, "", checked)}
}
func (menu) SetTitle(title, tooltip string) {
	if runtime.GOOS == "darwin" {
		systray.SetTitle(title) // next to the icon; Windows and Linux show only the icon
	}
	systray.SetTooltip(tooltip)
}

type item struct{ *systray.MenuItem }

func (i item) Sub(title, tooltip string) tray.Item {
	return item{i.AddSubMenuItem(title, tooltip)}
}
func (i item) Disable() { i.MenuItem.Disable() }
func (i item) OnClick(f func()) {
	go func() {
		for range i.ClickedCh {
			f()
		}
	}()
}
