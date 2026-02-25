export ZSH="/opt/oh-my-zsh"
export BUN_INSTALL="/opt/bun"
export NVM_DIR="/opt/nvm"
export GITSTATUS_CACHE_DIR="/opt/gitstatus"
export PATH="$BUN_INSTALL/bin:$BUN_INSTALL/install/global/node_modules/.bin:$PATH"
[ -s "$NVM_DIR/nvm.sh" ] && source "$NVM_DIR/nvm.sh"
POWERLEVEL9K_DISABLE_CONFIGURATION_WIZARD=true
ZSH_THEME="powerlevel10k/powerlevel10k"
[ -f /app/p10k.zsh ] && source /app/p10k.zsh
[ -f ~/.p10k.zsh ] && source ~/.p10k.zsh
plugins=(git)
source "$ZSH/oh-my-zsh.sh"
alias codex-yolo='codex --dangerously-bypass-approvals-and-sandbox'
