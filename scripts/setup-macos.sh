#!/usr/bin/env bash
# Sets up a Mac to build, test and run Meld, as scripts/setup-linux.sh does a Linux container. Meld is
# built and checked on Windows and Linux; macOS is not a supported platform yet, so this is the first
# step of finding out what it needs there, not a promise that everything works.
#
# It installs what is missing and skips what is there, so it is safe to run again:
#   - Go and Bun through Homebrew, when not on the PATH (a Go older than go.mod's downloads go.mod's
#     toolchain itself at the first build);
#   - wails3 at go.mod's Wails version (`make dev`, `make dev-page`, `make bindings`) and
#     golangci-lint v2 (`make check` skips the lint without it);
#   - the frontend's packages (`bun install --frozen-lockfile`).
#
# It needs the Xcode Command Line Tools (the desktop window is cgo over Cocoa and WebKit) and
# Homebrew, and installs neither: it says how and stops.
#
# Afterwards:
#   make dev-page   the app as a browser page at http://localhost:34115 (no native window needed)
#   make dev        the desktop window, with hot reload
#   make test       the Go and frontend unit tests
#   WOW335_DIR=/path/to/a/3.3.5a/client go test ./internal/... -run WotLK   the 3.3.5a client tests
# The probes find Chrome through CHROME:
#   CHROME="/Applications/Google Chrome.app/Contents/MacOS/Google Chrome" make probe-smoke
#
# Usage: scripts/setup-macos.sh [--no-lint]
set -euo pipefail

lint=1
for arg in "$@"; do
	case "$arg" in
		--no-lint) lint=0 ;;
		-h | --help)
			sed -n '2,24p' "$0"
			exit 0
			;;
		*)
			echo "setup-macos: unknown option $arg" >&2
			exit 2
			;;
	esac
done

if [ "$(uname -s)" != Darwin ]; then
	echo "setup-macos: not macOS, nothing to do"
	exit 0
fi

root=$(cd "$(dirname "$0")/.." && pwd)
log() { echo "setup-macos: $*"; }

# Adds a folder to this script's PATH, and says so when the user's shell does not have it.
missing_paths=
add_path() {
	case ":$PATH:" in
		*":$1:"*) ;;
		*)
			export PATH="$1:$PATH"
			missing_paths="$missing_paths $1"
			;;
	esac
}

if ! xcode-select -p >/dev/null 2>&1; then
	log "the Xcode Command Line Tools are missing: run 'xcode-select --install', then this script again"
	exit 1
fi

# Homebrew is not on the PATH of a shell that has not run its shellenv (a script, an agent).
for prefix in /opt/homebrew /usr/local; do
	if [ -x "$prefix/bin/brew" ]; then
		add_path "$prefix/bin"
		break
	fi
done
need_brew() {
	if ! command -v brew >/dev/null; then
		log "$1 is missing and so is Homebrew: install Homebrew (https://brew.sh) or $1 by hand, then run this again"
		exit 1
	fi
}

# Go. A Go older than go.mod's fetches that toolchain on its own (GOTOOLCHAIN=auto, the default).
if ! command -v go >/dev/null; then
	need_brew Go
	log "installing Go"
	brew install go
fi
gover=$(sed -n 's/^go \([0-9.]*\).*/\1/p' "$root/go.mod")
log "go.mod asks for Go $gover; this is $(go env GOVERSION)"
add_path "$(go env GOPATH)/bin"

# Bun.
if ! command -v bun >/dev/null; then
	if [ -x "$HOME/.bun/bin/bun" ]; then
		add_path "$HOME/.bun/bin"
	else
		need_brew Bun
		log "installing Bun"
		brew install oven-sh/bun/bun
	fi
fi

# wails3, at the Wails version go.mod pins.
wailsver=$(sed -n 's#^[[:space:]]*github.com/wailsapp/wails/v3 \(v[^ ]*\).*#\1#p' "$root/go.mod")
if ! command -v wails3 >/dev/null || ! wails3 version 2>/dev/null | grep -qF "${wailsver#v}"; then
	log "installing wails3 $wailsver"
	(cd "$root" && go install "github.com/wailsapp/wails/v3/cmd/wails3@$wailsver")
fi

if [ $lint = 1 ] && ! command -v golangci-lint >/dev/null; then
	log "installing golangci-lint v2"
	(cd "$root" && go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest)
fi

log "bun install"
(cd "$root/frontend" && bun install --frozen-lockfile)

log "done: go $(go env GOVERSION), bun $(bun --version), $(wails3 version 2>/dev/null | head -n 1)"
if [ -n "$missing_paths" ]; then
	log "add these to your shell's PATH (~/.zshrc), which does not have them:$missing_paths"
fi
log "next: make dev-page, then open http://localhost:34115"
