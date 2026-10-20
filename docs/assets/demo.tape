# Demo recording for the README. Render with: make demo  (requires https://github.com/charmbracelet/vhs)
Output docs/assets/demo.gif

Set Shell "bash"
Set FontSize 16
Set Width 1200
Set Height 640
Set Theme "Catppuccin Mocha"
Set TypingSpeed 60ms

Hide
Type "python3 -m http.server 8000 --bind 127.0.0.1 >/dev/null 2>&1 &" Enter
Type "clear" Enter
Show

Type "portcheck 8000" Sleep 400ms Enter
Sleep 2s

Type "portcheck free 3000-3999 -n 3" Sleep 400ms Enter
Sleep 2s

Type "portcheck" Sleep 400ms Enter
Sleep 3s
Type "/8000" Sleep 800ms Enter
Sleep 1s
Type "x" Sleep 1500ms
Type "y" Sleep 3s
Type "?" Sleep 3s
Type "q" Sleep 500ms
Type "q"
Sleep 1s
