package cmdports

import "strings"

// opToken stands for a command separator (;, &, |, &&, ||, newline, parentheses).
const opToken = "\x00"

// split breaks a command line into words the way a POSIX shell roughly would:
// single and double quotes group, a backslash escapes the next character
// (outside single quotes), and unquoted ; & | ( ) and newlines separate
// commands. It doesn't expand variables or globs.
func split(s string) []string {
	var words []string
	var cur strings.Builder
	inWord := false
	flush := func() {
		if inWord {
			words = append(words, cur.String())
			cur.Reset()
			inWord = false
		}
	}
	var quote rune
	escaped := false
	for _, r := range s {
		switch {
		case escaped:
			cur.WriteRune(r)
			escaped = false
		case quote == '\'':
			if r == '\'' {
				quote = 0
			} else {
				cur.WriteRune(r)
			}
		case r == '\\':
			escaped, inWord = true, true
		case quote == '"':
			if r == '"' {
				quote = 0
			} else {
				cur.WriteRune(r)
			}
		case r == '\'' || r == '"':
			quote, inWord = r, true
		case r == ' ' || r == '\t':
			flush()
		case strings.ContainsRune(";&|()\n", r):
			flush()
			if len(words) == 0 || words[len(words)-1] != opToken {
				words = append(words, opToken)
			}
		default:
			cur.WriteRune(r)
			inWord = true
		}
	}
	flush()
	return words
}

// segments splits words at command separators, dropping empty commands.
func segments(words []string) [][]string {
	var out [][]string
	start := 0
	for i := 0; i <= len(words); i++ {
		if i == len(words) || words[i] == opToken {
			if i > start {
				out = append(out, words[start:i])
			}
			start = i + 1
		}
	}
	return out
}
