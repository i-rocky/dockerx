FROM debian:bookworm-slim
LABEL authors="rocky"

ENV DEBIAN_FRONTEND=noninteractive \
    BUN_INSTALL=/opt/bun \
    NVM_DIR=/opt/nvm \
    UV_INSTALL_DIR=/opt/uv \
    CARGO_HOME=/opt/cargo \
    RUSTUP_HOME=/opt/rustup \
    GOPATH=/opt/go \
    ZSH=/opt/oh-my-zsh \
    GITSTATUS_CACHE_DIR=/opt/gitstatus \
    POWERLEVEL9K_DISABLE_CONFIGURATION_WIZARD=true \
    TERM=xterm-256color \
    PATH=/opt/bun/bin:/opt/bun/install/global/node_modules/.bin:/opt/uv:/opt/cargo/bin:/opt/go/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin

RUN apt-get update && apt-get install -y --no-install-recommends \
    bash \
    build-essential \
    gcc \
    g++ \
    make \
    cmake \
    ninja-build \
    git \
    curl \
    wget \
    axel \
    zsh \
    htop \
    sudo \
    libnss-wrapper \
    unzip \
    golang-go \
    nodejs \
    npm \
    ca-certificates \
  && rm -rf /var/lib/apt/lists/*

# Bun + Codex
RUN curl -fsSL https://bun.sh/install | bash \
  && /opt/bun/bin/bun add -g @openai/codex \
  && chmod -R a+rX /opt/bun

# nvm
RUN git clone --depth=1 https://github.com/nvm-sh/nvm.git "$NVM_DIR" \
  && chmod -R a+rX "$NVM_DIR"

# uv
RUN curl -LsSf https://astral.sh/uv/install.sh | env UV_INSTALL_DIR="$UV_INSTALL_DIR" INSTALLER_NO_MODIFY_PATH=1 sh \
  && chmod -R a+rX "$UV_INSTALL_DIR"

# Rust toolchain
RUN curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | env CARGO_HOME="$CARGO_HOME" RUSTUP_HOME="$RUSTUP_HOME" sh -s -- -y --profile minimal --default-toolchain stable \
  && chmod -R a+rX "$CARGO_HOME" "$RUSTUP_HOME"

# Oh-My-Zsh + Powerlevel10k
RUN git clone --depth=1 https://github.com/ohmyzsh/ohmyzsh.git "$ZSH" \
  && git clone --depth=1 https://github.com/romkatv/powerlevel10k.git "$ZSH/custom/themes/powerlevel10k" \
  && git -C "$ZSH/custom/themes/powerlevel10k" submodule update --init --depth=1 \
  && GITSTATUS_CACHE_DIR=/opt/gitstatus \
     "$ZSH/custom/themes/powerlevel10k/gitstatus/install" \
  && chmod -R a+rX /opt/gitstatus

COPY zshrc /etc/zsh/zshrc

COPY entrypoint.sh /usr/local/bin/entrypoint.sh
RUN chmod +x /usr/local/bin/entrypoint.sh

RUN printf 'ALL ALL=(ALL) NOPASSWD:ALL\n' > /etc/sudoers.d/00-nopasswd \
  && chmod 0440 /etc/sudoers.d/00-nopasswd

ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
CMD ["zsh"]
