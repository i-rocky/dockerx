FROM debian:bookworm-slim
LABEL authors="rocky"

ENV DEBIAN_FRONTEND=noninteractive \
    BUN_INSTALL=/opt/bun \
    ZSH=/opt/oh-my-zsh \
    GITSTATUS_CACHE_DIR=/opt/gitstatus \
    POWERLEVEL9K_DISABLE_CONFIGURATION_WIZARD=true \
    TERM=xterm-256color \
    PATH=/opt/bun/bin:/opt/bun/install/global/node_modules/.bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin

RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential \
    gcc \
    g++ \
    make \
    cmake \
    git \
    curl \
    wget \
    axel \
    zsh \
    htop \
    sudo \
    libnss-wrapper \
    unzip \
    nodejs \
    npm \
    ca-certificates \
  && rm -rf /var/lib/apt/lists/*

# Bun + Codex
RUN curl -fsSL https://bun.sh/install | bash \
  && /opt/bun/bin/bun add -g @openai/codex \
  && chmod -R a+rX /opt/bun

# Oh-My-Zsh + Powerlevel10k
RUN git clone --depth=1 https://github.com/ohmyzsh/ohmyzsh.git "$ZSH" \
  && git clone --depth=1 https://github.com/romkatv/powerlevel10k.git "$ZSH/custom/themes/powerlevel10k" \
  && git -C "$ZSH/custom/themes/powerlevel10k" submodule update --init --depth=1 \
  && GITSTATUS_CACHE_DIR=/opt/gitstatus \
     "$ZSH/custom/themes/powerlevel10k/gitstatus/install" \
  && chmod -R a+rX /opt/gitstatus \
  && mkdir -p /etc/zsh \
  && printf '%s\n' \
    'export ZSH="/opt/oh-my-zsh"' \
    'export BUN_INSTALL="/opt/bun"' \
    'export GITSTATUS_CACHE_DIR="/opt/gitstatus"' \
    'export PATH="$BUN_INSTALL/bin:$BUN_INSTALL/install/global/node_modules/.bin:$PATH"' \
    'POWERLEVEL9K_DISABLE_CONFIGURATION_WIZARD=true' \
    'ZSH_THEME="powerlevel10k/powerlevel10k"' \
    '[ -f /app/p10k.zsh ] && source /app/p10k.zsh' \
    '[ -f ~/.p10k.zsh ] && source ~/.p10k.zsh' \
    'plugins=(git)' \
    'source "$ZSH/oh-my-zsh.sh"' \
    > /etc/zsh/zshrc

RUN printf '%s\n' \
    '#!/bin/sh' \
    'set -eu' \
    'if [ "$(id -u)" -ne 0 ]; then' \
    '  USER=${USER:-dev}' \
    '  if [ -z "${HOME:-}" ] || [ "${HOME:-}" = "/" ]; then' \
    '    HOME="/tmp/home/$USER"' \
    '  fi' \
    '  if [ ! -w "$HOME" ]; then' \
    '    HOME="/tmp/home/$USER"' \
    '  fi' \
    '  mkdir -p "$HOME"' \
    '  if [ ! -f "$HOME/.zshrc" ]; then' \
    '    printf "%s\n" "source /etc/zsh/zshrc" > "$HOME/.zshrc"' \
    '  fi' \
    '  passwd_file=$(mktemp)' \
    '  group_file=$(mktemp)' \
    '  echo "${USER}:x:$(id -u):$(id -g):${USER} user:${HOME}:/bin/zsh" > "$passwd_file"' \
    '  echo "${USER}:x:$(id -g):" > "$group_file"' \
    '  export NSS_WRAPPER_PASSWD="$passwd_file"' \
    '  export NSS_WRAPPER_GROUP="$group_file"' \
    '  for p in /usr/lib/*/libnss_wrapper.so /usr/lib/libnss_wrapper.so; do' \
    '    if [ -r "$p" ]; then' \
    '      export LD_PRELOAD="$p${LD_PRELOAD:+:$LD_PRELOAD}"' \
    '      break' \
    '    fi' \
    '  done' \
    '  export USER HOME' \
    '  export XDG_CACHE_HOME="$HOME/.cache"' \
    'fi' \
    'exec "$@"' \
    > /usr/local/bin/entrypoint.sh \
  && chmod +x /usr/local/bin/entrypoint.sh

RUN printf 'ALL ALL=(ALL) NOPASSWD:ALL\n' > /etc/sudoers.d/00-nopasswd \
  && chmod 0440 /etc/sudoers.d/00-nopasswd

ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
CMD ["zsh"]
