# Tells Rex the shell's folder, so relative file names are clickable.
if [[ -n "$REX_SESSION" || -n "$REX_BLOCK" || "$TERM_PROGRAM" == rex ]]; then
  autoload -Uz add-zsh-hook
  _rdxt_report_dir() { printf '\e]7;file://%s%s\e\\' "${HOST}" "${PWD// /%20}" }
  add-zsh-hook chpwd _rdxt_report_dir
  _rdxt_report_dir
fi
# Typing a document's name opens it: notes.md + Enter.
alias -s md=rdxt markdown=rdxt dok=rdxt
