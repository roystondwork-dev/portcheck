package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/roystondwork-dev/portcheck/internal/nativehost"
	"github.com/roystondwork-dev/portcheck/internal/output"
)

// serveNativeHost answers native messages from a browser extension on the
// app's stdin/stdout (ADR-0005). Each request runs a portcheck command
// in-process with its output captured.
func serveNativeHost(ctx context.Context, app *App) int {
	run := func(ctx context.Context, args []string) ([]byte, []byte, int) {
		var stdout, stderr bytes.Buffer
		sub := *app
		sub.Stdin = strings.NewReader("") // stdin carries the browser's messages
		sub.Stdout, sub.Stderr = &stdout, &stderr
		sub.StdinTTY, sub.StdoutTTY, sub.StderrTTY = false, false, false
		code := Execute(ctx, &sub, args)
		return stdout.Bytes(), stderr.Bytes(), code
	}
	if err := nativehost.Serve(ctx, app.Stdin, app.Stdout, run); err != nil {
		fmt.Fprintf(app.Stderr, "portcheck: native messaging: %v\n", err)
		return ExitRuntime
	}
	return ExitOK
}

type browserFlags struct {
	browsers     []string
	extensionIDs []string
	path         string
}

func (c *cli) newBrowserCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "browser",
		Short: "Connect portcheck to the browser extension",
		Long: `Register portcheck with your browsers (Chrome, Edge, Chromium, Brave, Firefox)
so the portcheck browser extension can ask it which process serves a localhost
page, and stop it.

Registration is per user and needs no administrator rights. The browser can
only list ports, check one port and stop one process, with the same safety
guards as 'portcheck kill'.`,
		Example: `  portcheck browser install
  portcheck browser install --browser edge
  portcheck browser status
  portcheck browser uninstall`,
		Args: usageArgs(cobra.NoArgs),
	}
	cmd.AddCommand(c.newBrowserInstallCmd(), c.newBrowserUninstallCmd(), c.newBrowserStatusCmd())
	return cmd
}

func (c *cli) newBrowserInstallCmd() *cobra.Command {
	var f browserFlags
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Register portcheck with your browsers",
		Long: `Register portcheck as the native messaging host for the portcheck extension,
in every supported browser installed for this user (or those given with
--browser). Run it again after moving the portcheck binary.`,
		Args: usageArgs(cobra.NoArgs),
		RunE: func(*cobra.Command, []string) error { return c.runBrowserInstall(f) },
	}
	cmd.Flags().StringSliceVar(&f.browsers, "browser", nil, "browsers to register with: chrome, edge, chromium, brave, firefox (default: those installed)")
	cmd.Flags().StringSliceVar(&f.extensionIDs, "extension-id", nil, "also allow these extension IDs (Chromium) or add-on IDs (Firefox), e.g. a development build")
	cmd.Flags().StringVar(&f.path, "path", "", "portcheck binary to register (default: this one)")
	return cmd
}

func (c *cli) newBrowserUninstallCmd() *cobra.Command {
	var f browserFlags
	cmd := &cobra.Command{
		Use:   "uninstall",
		Short: "Remove portcheck's browser registration",
		Args:  usageArgs(cobra.NoArgs),
		RunE:  func(*cobra.Command, []string) error { return c.runBrowserUninstall(f) },
	}
	cmd.Flags().StringSliceVar(&f.browsers, "browser", nil, "browsers to unregister from (default: all)")
	return cmd
}

func (c *cli) newBrowserStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show which browsers portcheck is registered with",
		Args:  usageArgs(cobra.NoArgs),
		RunE:  func(*cobra.Command, []string) error { return c.runBrowserStatus() },
	}
}

func (c *cli) browserTargets() (nativehost.Env, []nativehost.Target, error) {
	env, err := c.app.BrowserEnv()
	if err != nil {
		return env, nil, err
	}
	targets, err := nativehost.Targets(env)
	return env, targets, err
}

func (c *cli) runBrowserInstall(f browserFlags) error {
	var extraChromium, extraGecko []string
	for _, id := range f.extensionIDs {
		switch {
		case validExtensionID(id):
			extraChromium = append(extraChromium, id)
		case validGeckoID(id):
			extraGecko = append(extraGecko, id)
		default:
			return usageError("invalid extension ID %q (Chromium: 32 letters a-p; Firefox: name@domain or {GUID})", id)
		}
	}
	env, targets, err := c.browserTargets()
	if err != nil {
		return err
	}
	selected, err := nativehost.Select(targets, f.browsers)
	if err != nil {
		return &ExitError{Code: ExitUsage, Err: err}
	}
	if len(selected) == 0 {
		return errors.New("found no supported browser (Chrome, Edge, Chromium, Brave, Firefox) for this user; use --browser to register anyway")
	}
	path, err := c.hostPath(f.path)
	if err != nil {
		return err
	}
	chromium := slices.Concat(nativehost.ExtensionIDs, extraChromium)
	gecko := slices.Concat(nativehost.GeckoIDs, extraGecko)
	for _, t := range selected {
		if err := nativehost.Install(env, t, nativehost.NewManifest(t.Browser, path, chromium, gecko)); err != nil {
			return err
		}
		fmt.Fprintf(c.app.Stderr, "Registered with %s.\n", t.Browser.Name)
	}
	return c.printBrowsers(env, selected)
}

func (c *cli) runBrowserUninstall(f browserFlags) error {
	env, targets, err := c.browserTargets()
	if err != nil {
		return err
	}
	selected := targets
	if len(f.browsers) > 0 {
		if selected, err = nativehost.Select(targets, f.browsers); err != nil {
			return &ExitError{Code: ExitUsage, Err: err}
		}
	}
	for _, t := range selected {
		if err := nativehost.Uninstall(env, t); err != nil {
			return err
		}
	}
	return c.printBrowsers(env, selected)
}

func (c *cli) runBrowserStatus() error {
	env, targets, err := c.browserTargets()
	if err != nil {
		return err
	}
	return c.printBrowsers(env, targets)
}

func (c *cli) printBrowsers(env nativehost.Env, targets []nativehost.Target) error {
	rows := make([]output.BrowserJSON, 0, len(targets))
	for _, t := range targets {
		m, err := nativehost.Installed(env, t)
		if err != nil {
			return err
		}
		row := output.BrowserJSON{Browser: t.Browser.ID, Name: t.Browser.Name, Detected: t.Detected, Manifest: t.Manifest}
		if m != nil {
			row.Registered = true
			row.Path = m.Path
			row.Extensions = m.AllowedIDs()
		}
		rows = append(rows, row)
	}
	return c.printer.Print(output.BrowserDoc(nativehost.HostName, rows))
}

// hostPath is the binary the registration starts: --path, or this binary
// (preferring its location on PATH, which survives package upgrades).
func (c *cli) hostPath(flag string) (string, error) {
	if flag != "" {
		abs, err := filepath.Abs(flag)
		if err != nil {
			return "", fmt.Errorf("resolve --path: %w", err)
		}
		return abs, nil
	}
	exe, err := c.app.Executable()
	if err != nil {
		return "", fmt.Errorf("find the portcheck binary: %w (use --path)", err)
	}
	onPath, _ := c.app.LookPath("portcheck")
	if onPath != "" {
		onPath, _ = filepath.Abs(onPath)
	}
	return nativehost.HostPath(exe, onPath), nil
}

// geckoID matches Firefox add-on IDs: email-like (name@domain) or a {GUID}.
var geckoID = regexp.MustCompile(`^([A-Za-z0-9._+-]*@[A-Za-z0-9.-]+|\{[0-9A-Fa-f]{8}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{12}\})$`)

// validGeckoID reports whether id looks like a Firefox add-on ID.
func validGeckoID(id string) bool { return geckoID.MatchString(id) }

// validExtensionID reports whether id looks like a Chromium extension ID.
func validExtensionID(id string) bool {
	if len(id) != 32 {
		return false
	}
	for _, r := range id {
		if r < 'a' || r > 'p' {
			return false
		}
	}
	return true
}
