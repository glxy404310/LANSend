#!/bin/bash
set -eu
cd "$(dirname "$0")"
target="$HOME/Library/Application Support/LANSend"
services="$HOME/Library/Services"
domain="gui/$(id -u)"
agent="$HOME/Library/LaunchAgents/local.lansend.receiver.plist"
engine="$PWD/engine"
mkdir -p "$target" "$services" "$HOME/Library/LaunchAgents"
"$engine" init
if cmp -s "$engine" "$target/lansend" && [[ -d "$target/LANSend.app" && -d "$services/发送到 Windows.workflow" ]] && "$engine" ready; then exit 0; fi
launchctl bootout "$domain/local.lansend.receiver" 2>/dev/null || true
install -m 755 "$engine" "$target/lansend"
ditto "Receipts.app" "$target/LANSend.app"
ditto '发送到 Windows.workflow' "$services/发送到 Windows.workflow"
plutil -create xml1 "$agent"
plutil -insert Label -string local.lansend.receiver "$agent"
plutil -insert ProgramArguments -array "$agent"
plutil -insert ProgramArguments.0 -string "$target/lansend" "$agent"
plutil -insert ProgramArguments.1 -string receive "$agent"
plutil -insert RunAtLoad -bool YES "$agent"
launchctl bootstrap "$domain" "$agent"
"$engine" ready
/usr/bin/open -g -a "$target/LANSend.app"
/System/Library/CoreServices/pbs -flush 2>/dev/null || true
