#!/bin/bash
set -eu
cd "$(dirname "$0")"
app="$(cd ../.. && pwd -P)"
target="$HOME/Library/Application Support/LANSend"
services="$HOME/Library/Services"
agents="$HOME/Library/LaunchAgents"
agent="$agents/local.lansend.receiver.plist"
domain="gui/$(id -u)"
mkdir -p "$target" "$services" "$agents"
"$PWD/engine" init
# Finder's existing Quick Action calls this copy for sending only.
if ! cmp -s engine "$target/lansend"; then
  temp=$(mktemp "$target/.engine.XXXXXXXX")
  install -m 755 engine "$temp"
  mv -f "$temp" "$target/lansend"
fi
rm -rf "$services/发送到 Windows.workflow"
ditto '用 LANSend 发送.workflow' "$services/用 LANSend 发送.workflow"
/System/Library/CoreServices/pbs -flush >/dev/null 2>&1 || true
# Login opens the application. Only the application launches the receiver.
temp=$(mktemp "$agents/.lansend.XXXXXXXX")
trap 'rm -f "$temp"' EXIT
plutil -create xml1 "$temp"
plutil -insert Label -string local.lansend.receiver "$temp"
plutil -insert ProgramArguments -array "$temp"
plutil -insert ProgramArguments.0 -string /usr/bin/open "$temp"
plutil -insert ProgramArguments.1 -string -g "$temp"
plutil -insert ProgramArguments.2 -string -a "$temp"
plutil -insert ProgramArguments.3 -string "$app" "$temp"
plutil -insert ProgramArguments.4 -string --args "$temp"
plutil -insert ProgramArguments.5 -string --background-receiver "$temp"
plutil -insert RunAtLoad -bool YES "$temp"
if ! cmp -s "$temp" "$agent"; then
  if [[ -f "$agent" ]]; then
    launchctl bootout "$domain/local.lansend.receiver" 2>/dev/null || true
  fi
  mv -f "$temp" "$agent"
fi
# launchd loads the plist at the next login. This instance serves immediately.
