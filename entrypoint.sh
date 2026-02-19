#!/bin/sh
set -eu

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
  export XDG_CACHE_HOME="$HOME/.cache"
fi

exec "$@"
