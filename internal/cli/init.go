package cli

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

func (c *cli) newInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init <zsh|bash|fish|powershell>",
		Short: `Shell integration: explain "address already in use" after a command fails`,
		Long: `Print a snippet for your shell's startup file. With it, whenever a command
fails, portcheck checks the ports that command line listens on (its --port,
-p 8080:80, PORT=…, or a known dev server's default) and, if something holds
one, says what and how to free it:

  portcheck: port 3000 is held by node (pid 41852), started by npm (41850) ← zsh (900), in my-shop
  portcheck: stop it: portcheck kill 3000

It prints nothing otherwise. Add one line to your shell's startup file:

  zsh   (~/.zshrc)        eval "$(portcheck init zsh)"
  bash  (~/.bashrc)       eval "$(portcheck init bash)"
  fish  (config.fish)     portcheck init fish | source
  PowerShell ($PROFILE)   portcheck init powershell | Out-String | Invoke-Expression`,
		Example:   `  echo 'eval "$(portcheck init zsh)"' >> ~/.zshrc`,
		Args:      usageArgs(cobra.ExactArgs(1)),
		ValidArgs: []string{"zsh", "bash", "fish", "powershell"},
		RunE: func(cmd *cobra.Command, args []string) error {
			snippet, ok := shellSnippets[strings.ToLower(args[0])]
			if !ok {
				return usageError("unknown shell %q (want zsh, bash, fish or powershell)", args[0])
			}
			exe := c.hookExecutable()
			_, err := io.WriteString(cmd.OutOrStdout(), strings.ReplaceAll(snippet.text, "@EXE@", snippet.quote(exe)))
			if err != nil {
				return fmt.Errorf("write snippet: %w", err)
			}
			return nil
		},
	}
}

// hookExecutable is how the snippet runs portcheck: by name when it is on
// PATH (so the hook keeps working after upgrades move the binary, as Homebrew
// does), otherwise by the running binary's absolute path.
func (c *cli) hookExecutable() string {
	exe, err := c.app.Executable()
	if err != nil {
		return "portcheck"
	}
	if onPath, err := c.app.LookPath("portcheck"); err == nil && sameFile(onPath, exe) {
		return "portcheck"
	}
	return exe
}

// sameFile reports whether two paths name the same file, following symlinks.
func sameFile(a, b string) bool {
	ra, errA := filepath.EvalSymlinks(a)
	rb, errB := filepath.EvalSymlinks(b)
	return errA == nil && errB == nil && ra == rb
}

type snippet struct {
	text  string
	quote func(string) string
}

// posixQuote quotes s for sh, bash and zsh.
func posixQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// fishQuote quotes s for fish.
func fishQuote(s string) string {
	return "'" + strings.NewReplacer(`\`, `\\`, `'`, `\'`).Replace(s) + "'"
}

// psQuote quotes s for PowerShell.
func psQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }

// The snippets skip successful, interrupted (130), suspended (148), not found
// (127) and not executable (126) commands, and portcheck itself; explain
// checks the rest and prints nothing unless a port it reads is held.
var shellSnippets = map[string]snippet{
	"zsh": {quote: posixQuote, text: `# portcheck shell integration (zsh): https://roystondwork-dev.github.io/portcheck/guide/shell/
autoload -Uz add-zsh-hook
typeset -g __portcheck_cmd=''
__portcheck_preexec() { __portcheck_cmd=$1; }
__portcheck_precmd() {
  local code=$? cmd=$__portcheck_cmd
  __portcheck_cmd=''
  case $code in 0|126|127|130|148) return ;; esac
  [[ -z $cmd || $cmd == portcheck* ]] && return
  @EXE@ explain --exit "$code" -- "$cmd"
}
add-zsh-hook preexec __portcheck_preexec
add-zsh-hook precmd __portcheck_precmd
`},
	"bash": {quote: posixQuote, text: `# portcheck shell integration (bash): https://roystondwork-dev.github.io/portcheck/guide/shell/
__portcheck_seen=''
__portcheck_prompt() {
  local code=$? entry cmd
  entry=$(HISTTIMEFORMAT='' builtin history 1)
  if [[ $entry != "$__portcheck_seen" ]]; then
    __portcheck_seen=$entry
    case $code in 0|126|127|130|148) ;; *)
      cmd=${entry#"${entry%%[![:space:]]*}"}
      cmd=${cmd#*[[:space:]]}
      cmd=${cmd#"${cmd%%[![:space:]]*}"}
      [[ -n $cmd && $cmd != portcheck* ]] && @EXE@ explain --exit "$code" -- "$cmd"
    ;; esac
  fi
  return $code
}
__portcheck_seen=$(HISTTIMEFORMAT='' builtin history 1)
case ";${PROMPT_COMMAND:-};" in
  *";__portcheck_prompt;"*) ;;
  *) PROMPT_COMMAND="__portcheck_prompt${PROMPT_COMMAND:+;$PROMPT_COMMAND}" ;;
esac
`},
	"fish": {quote: fishQuote, text: `# portcheck shell integration (fish): https://roystondwork-dev.github.io/portcheck/guide/shell/
function __portcheck_postexec --on-event fish_postexec
    set -l code $status
    contains -- $code 0 126 127 130 148; and return
    set -l cmd (string trim -- $argv[1])
    test -z "$cmd"; and return
    string match -q 'portcheck*' -- $cmd; and return
    @EXE@ explain --exit $code -- $cmd
end
`},
	"powershell": {quote: psQuote, text: `# portcheck shell integration (PowerShell): https://roystondwork-dev.github.io/portcheck/guide/shell/
$global:__portcheckPrompt = $function:prompt
$global:__portcheckSeen = (Get-History -Count 1).Id
function global:prompt {
    $ok = $?
    $code = $global:LASTEXITCODE
    $h = Get-History -Count 1
    if ($h -and $h.Id -ne $global:__portcheckSeen) {
        $global:__portcheckSeen = $h.Id
        if (-not $ok -and $code -and @(0, 126, 127, 130, 148) -notcontains $code -and $h.CommandLine -notlike 'portcheck*') {
            & @EXE@ explain --exit $code -- $h.CommandLine
        }
    }
    $global:LASTEXITCODE = $code
    & $global:__portcheckPrompt
}
`},
}
