#!/bin/sh
# Generates shell completions and man pages bundled into release archives and
# packages. Run by GoReleaser's before hooks; safe to run locally.
set -eu

rm -rf completions manpages
mkdir -p completions manpages

for sh in bash zsh fish powershell; do
	go run ./cmd/portcheck completion "$sh" > "completions/portcheck.$sh"
done

go run ./internal/cli/gendocs -out "" -man manpages
gzip -9n manpages/*.1
