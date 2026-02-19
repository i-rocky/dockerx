#!/bin/sh
set -eu

config_count() {
  count=${DOCKERX_CONFIG_COUNT:-0}
  case "$count" in
    ''|*[!0-9]*)
      count=0
      ;;
  esac
  printf '%s\n' "$count"
}

resolve_target_path() {
  target=$1
  case "$target" in
    /home/dev)
      printf '%s\n' "$HOME"
      ;;
    /home/dev/*)
      printf '%s/%s\n' "$HOME" "${target#/home/dev/}"
      ;;
    *)
      printf '%s\n' "$target"
      ;;
  esac
}

copy_staged_configs() {
  count=$(config_count)
  i=0
  while [ "$i" -lt "$count" ]; do
    eval "src=\${DOCKERX_CONFIG_SRC_$i:-}"
    eval "dst=\${DOCKERX_CONFIG_DST_$i:-}"
    i=$((i + 1))

    if [ -z "$src" ] || [ -z "$dst" ] || [ ! -e "$src" ]; then
      continue
    fi

    resolved_dst=$(resolve_target_path "$dst")
    mkdir -p "$(dirname "$resolved_dst")"
    rm -rf "$resolved_dst"
    cp -a "$src" "$resolved_dst"
  done
}

print_staged_config_diffs() {
  count=$(config_count)
  i=0
  changed=0
  had_error=0

  while [ "$i" -lt "$count" ]; do
    eval "src=\${DOCKERX_CONFIG_SRC_$i:-}"
    eval "dst=\${DOCKERX_CONFIG_DST_$i:-}"
    i=$((i + 1))

    if [ -z "$src" ] || [ -z "$dst" ] || [ ! -e "$src" ]; then
      continue
    fi

    resolved_dst=$(resolve_target_path "$dst")
    tmpdiff=$(mktemp)
    set +e
    if [ -d "$src" ] || [ -d "$resolved_dst" ]; then
      diff -ruN --no-dereference -x tmp "$src" "$resolved_dst" >"$tmpdiff" 2>&1
    else
      diff -uN "$src" "$resolved_dst" >"$tmpdiff" 2>&1
    fi
    status=$?
    set -e

    if [ "$status" -eq 0 ]; then
      rm -f "$tmpdiff"
      continue
    fi

    if [ "$status" -eq 1 ]; then
      if [ "$changed" -eq 0 ]; then
        printf "\n=== Config Diffs (mounted snapshot vs container copy) ===\n"
      fi
      changed=1
      printf "\n--- %s ---\n" "$resolved_dst"
      cat "$tmpdiff"
    else
      had_error=1
      printf "\n[warn] failed to diff %s and %s\n" "$src" "$resolved_dst" >&2
      cat "$tmpdiff" >&2
    fi

    rm -f "$tmpdiff"
  done

  if [ "$count" -gt 0 ] && [ "$changed" -eq 0 ] && [ "$had_error" -eq 0 ]; then
    printf "\nNo config changes detected.\n"
  fi
}

if [ "$(id -u)" -ne 0 ]; then
  USER=${USER:-dev}

  if [ -z "${HOME:-}" ] || [ "${HOME:-}" = "/" ]; then
    HOME="/tmp/home/$USER"
  fi

  if [ ! -w "$HOME" ]; then
    HOME="/tmp/home/$USER"
  fi

  mkdir -p "$HOME"

  if [ ! -f "$HOME/.zshrc" ]; then
    printf "%s\n" "source /etc/zsh/zshrc" > "$HOME/.zshrc"
  fi

  passwd_file=$(mktemp)
  group_file=$(mktemp)
  echo "${USER}:x:$(id -u):$(id -g):${USER} user:${HOME}:/bin/zsh" > "$passwd_file"
  echo "${USER}:x:$(id -g):" > "$group_file"
  export NSS_WRAPPER_PASSWD="$passwd_file"
  export NSS_WRAPPER_GROUP="$group_file"

  for p in /usr/lib/*/libnss_wrapper.so /usr/lib/libnss_wrapper.so; do
    if [ -r "$p" ]; then
      export LD_PRELOAD="$p${LD_PRELOAD:+:$LD_PRELOAD}"
      break
    fi
  done

  export USER HOME
  export CODEX_HOME="$HOME/.codex"
  export XDG_CACHE_HOME="$HOME/.cache"
fi

copy_staged_configs

cmd_status=0
"$@" || cmd_status=$?

print_staged_config_diffs
exit "$cmd_status"
