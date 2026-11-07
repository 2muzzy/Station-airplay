#!/bin/sh
# Run by YAM after the package payload and links are staged. Luna supervises
# the plugin, so "installed" is not "working": a YAM plugin stays disabled
# until it is enabled in Luna's own store.
#
# Luna and YAM share /tmp/yam/yam.lock. This hook runs while YAM holds that
# lock, so a synchronous Luna API call can block until YAM's postinstall
# timeout (60s) and make the whole install roll back. Hand the enable to a
# detached worker and return immediately.
#
# The worker must outlive the hook: YAM owns the hook's process group and may
# kill it when the transaction ends. setsid detaches it into its own session;
# nohup covers the signal case where setsid is missing.
#
# Stock-Luna behaviours that shape this hook, all verified on real devices:
#   * mutating endpoints accept only a JSON body; a form POST is answered 403;
#   * wget does not print a 4xx body, so errors can only be read from the
#     status line, never by matching the response text;
#   * a failed enable still saves service_enabled = true, so the plugin looks
#     configured while it never starts - the flag must be fixed first;
#   * a stock build ships allow_plugin_binaries = false and refuses to exec the
#     receiver;
#   * the device wget is BusyBox: no PUT, no --method, no --body-file. Only a
#     GNU wget can flip the setting through the API, so the worker falls back
#     to editing settings.toml and restarting Luna with its own command line;
#   * POST /api/plugins/rescan during a YAM transaction blocks on yam.lock, so
#     it only runs once YAM has exited, and only if the package is not visible.
set -u
URL=http://127.0.0.1:8099
PLUGIN=airplay.airplay-luna
LOG=/tmp/airplay-postinstall.log
WORKER=/tmp/airplay-enable-worker.sh

if ! command -v wget >/dev/null 2>&1; then
  echo "airplay: wget not found; enable $PLUGIN in Luna manually" >&2
  exit 0
fi

cat > "$WORKER" <<'EOS'
#!/bin/sh
set -u
URL=$1
PLUGIN=$2
LOG=$3

log() {
  echo "$(date '+%Y-%m-%d %H:%M:%S') $*" >>"$LOG"
}

api_get() {
  wget -q -O - -T 5 "$URL$1" 2>/dev/null
}

api_post_status() {
  wget -S -O /dev/null -T 20 --header='Content-Type: application/json' \
    --post-data='{}' "$URL$1" 2>&1 \
    | sed -n 's|.*HTTP/1\.[01] \([0-9][0-9][0-9]\).*|\1|p' | head -n 1
}

api_post() {
  wget -q -O - -T 20 --header='Content-Type: application/json' \
    --post-data='{}' "$URL$1" 2>/dev/null
}

have_pidof=0
command -v pidof >/dev/null 2>&1 && have_pidof=1

running() {
  [ "$have_pidof" = 1 ] && pidof station-airplay >/dev/null 2>&1
}

flag_state() {
  api_get /api/settings | sed -n 's/.*"allow_plugin_binaries":\([a-z]*\).*/\1/p' | head -n 1
}

luna_pid() {
  [ "$have_pidof" = 1 ] && pidof luna
}

luna_cmdline() {
  pid=$(luna_pid)
  [ -n "$pid" ] || return 1
  tr '\0' ' ' < "/proc/$pid/cmdline" 2>/dev/null | sed 's/ *$//'
}

luna_data_dir() {
  line=$(luna_cmdline) || return 1
  dir=$(printf '%s\n' "$line" | sed -n 's/.*--data-dir \([^ ]*\).*/\1/p')
  [ -n "$dir" ] && printf '%s\n' "$dir"
}

# BusyBox wget cannot PUT, so the API route only exists on a GNU build.
api_allow_binaries() {
  settings=$(api_get /api/settings) || return 1
  case "$settings" in
    *'"allow_plugin_binaries":true'*) return 0 ;;
    *'"allow_plugin_binaries":false'*) ;;
    *) return 1 ;;
  esac
  patched=$(printf '%s' "$settings" | sed 's/"allow_plugin_binaries":false/"allow_plugin_binaries":true/')
  printf '%s' "$patched" > /tmp/airplay-settings.json
  wget -q -O /dev/null -T 10 --method=PUT \
    --header='Content-Type: application/json' \
    --body-file=/tmp/airplay-settings.json "$URL/api/settings" 2>/dev/null
  [ "$(flag_state)" = true ]
}

# Last resort, and the only route on BusyBox: write the setting where Luna
# reads it and restart Luna with the command line it was started with.
file_allow_binaries() {
  data=$(luna_data_dir) || return 1
  settings="$data/settings.toml"
  line=$(luna_cmdline) || return 1
  pid=$(luna_pid)
  [ -n "$pid" ] || return 1

  mkdir -p "$data"
  if [ -f "$settings" ] && grep -q '^allow_plugin_binaries' "$settings"; then
    sed 's/^allow_plugin_binaries.*/allow_plugin_binaries = true/' "$settings" > "$settings.new" \
      && cat "$settings.new" > "$settings" && rm -f "$settings.new"
  else
    { printf 'allow_plugin_binaries = true\n'; cat "$settings" 2>/dev/null || true; } > "$settings.new" \
      && cat "$settings.new" > "$settings" && rm -f "$settings.new"
  fi
  grep -q '^allow_plugin_binaries = true' "$settings" || return 1
  log "settings.toml updated; restarting Luna to apply it"

  kill -TERM "$pid" 2>/dev/null
  i=0
  while [ "$i" -lt 20 ] && [ -n "$(luna_pid)" ]; do i=$((i + 1)); sleep 1; done
  [ -z "$(luna_pid)" ] || kill -KILL "$(luna_pid)" 2>/dev/null
  sleep 1

  # Same argv, same data dir, detached from this worker.
  setsid sh -c "exec $line" </dev/null >>/tmp/luna.log 2>&1 &
  i=0
  while [ "$i" -lt 60 ]; do
    api_get /api/device >/dev/null 2>&1 && break
    i=$((i + 1))
    sleep 2
  done
  [ "$(flag_state)" = true ]
}

log "worker started"

i=0
while [ "$i" -lt 40 ]; do
  api_get /api/device >/dev/null 2>&1 && break
  i=$((i + 1))
  sleep 2
done

# YAM still owns yam.lock while the transaction finishes. A Luna call made in
# that window (rescan in particular) blocks until YAM exits, so wait it out.
if [ "$have_pidof" = 1 ]; then
  i=0
  while [ "$i" -lt 45 ] && pidof yam >/dev/null 2>&1; do
    i=$((i + 1))
    sleep 2
  done
fi
sleep 3

# A stock Luna ships allow_plugin_binaries = false. Enable rejects the plugin
# in that case but still records service_enabled = true, so it has to be
# fixed before the first enable.
if [ "$(flag_state)" = true ]; then
  log "allow_plugin_binaries is on"
elif api_allow_binaries; then
  log "allow_plugin_binaries turned on through the API"
elif file_allow_binaries; then
  log "allow_plugin_binaries turned on through settings.toml"
else
  log "could not turn on allow_plugin_binaries; the plugin cannot start until it is enabled in the Luna settings"
fi

visible() {
  api_get /api/plugins 2>/dev/null | grep -q "$PLUGIN"
}

if ! visible; then
  api_post /api/plugins/rescan >/dev/null 2>&1
  sleep 2
  visible || log "$PLUGIN is still not listed after a rescan"
fi

i=0
while [ "$i" -lt 20 ]; do
  [ "$i" -gt 0 ] && api_post /api/plugins/rescan >/dev/null 2>&1

  status=$(api_post_status "/api/plugins/$PLUGIN/service/enable")
  [ "$i" -lt 3 ] && log "enable attempt $((i + 1)) answered ${status:-nothing}"

  # One enable restarts the plugin: give it a full minute before retrying.
  j=0
  while [ "$j" -lt 20 ]; do
    sleep 3
    if [ "$have_pidof" = 0 ] || running; then
      log "enabled and started $PLUGIN"
      exit 0
    fi
    j=$((j + 1))
  done

  i=$((i + 1))
  sleep 5
done

log "could not start $PLUGIN after about 20 minutes; the package is installed but disabled - check the Luna plugin log"
EOS

chmod +x "$WORKER" 2>/dev/null
if command -v setsid >/dev/null 2>&1; then
  setsid sh "$WORKER" "$URL" "$PLUGIN" "$LOG" </dev/null >>"$LOG" 2>&1 &
elif command -v nohup >/dev/null 2>&1; then
  nohup sh "$WORKER" "$URL" "$PLUGIN" "$LOG" </dev/null >>"$LOG" 2>&1 &
else
  sh "$WORKER" "$URL" "$PLUGIN" "$LOG" </dev/null >>"$LOG" 2>&1 &
fi

echo "airplay: scheduled $PLUGIN start in a detached worker"
# Let the worker exec into its own session before this process group is killed.
sleep 1
exit 0
