#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
SKILL_SOURCE="$PROJECT_ROOT/.claude/skills/panecom.md"

if [ ! -f "$SKILL_SOURCE" ]; then
  echo "error: skill source not found: $SKILL_SOURCE" >&2
  exit 1
fi

# Extract the content without YAML frontmatter for non-Claude agents
skill_body() {
  awk 'BEGIN{n=0} /^---$/{n++; next} n>=2{print}' "$SKILL_SOURCE"
}

deployed=()

# ── Claude Code (global) ──
claude_global="$HOME/.claude/skills/panecom.md"
mkdir -p "$(dirname "$claude_global")"
cp "$SKILL_SOURCE" "$claude_global"
deployed+=("Claude (global): $claude_global")

# ── Claude Code (project) ──
# Already exists at source, nothing to do
deployed+=("Claude (project): $SKILL_SOURCE")

# ── Codex CLI ──
# Codex reads AGENTS.md in the project root
agents_md="$PROJECT_ROOT/AGENTS.md"
marker_start="<!-- panecom-skill:start -->"
marker_end="<!-- panecom-skill:end -->"

panecom_section="${marker_start}
$(skill_body)
${marker_end}"

if [ -f "$agents_md" ]; then
  if grep -q "$marker_start" "$agents_md"; then
    # Replace existing section
    tmpfile=$(mktemp)
    awk -v start="$marker_start" -v end="$marker_end" -v new="$panecom_section" '
      $0 == start { print new; skip=1; next }
      $0 == end { skip=0; next }
      !skip { print }
    ' "$agents_md" > "$tmpfile"
    mv "$tmpfile" "$agents_md"
  else
    # Append
    printf "\n%s\n" "$panecom_section" >> "$agents_md"
  fi
else
  echo "$panecom_section" > "$agents_md"
fi
deployed+=("Codex (AGENTS.md): $agents_md")

# ── Codex global instructions ──
codex_global="$HOME/.codex/instructions.md"
if [ -d "$HOME/.codex" ] || mkdir -p "$HOME/.codex" 2>/dev/null; then
  if [ -f "$codex_global" ]; then
    if grep -q "$marker_start" "$codex_global"; then
      tmpfile=$(mktemp)
      awk -v start="$marker_start" -v end="$marker_end" -v new="$panecom_section" '
        $0 == start { print new; skip=1; next }
        $0 == end { skip=0; next }
        !skip { print }
      ' "$codex_global" > "$tmpfile"
      mv "$tmpfile" "$codex_global"
    else
      printf "\n%s\n" "$panecom_section" >> "$codex_global"
    fi
  else
    echo "$panecom_section" > "$codex_global"
  fi
  deployed+=("Codex (global): $codex_global")
fi

echo "panecom skill deployed:"
for d in "${deployed[@]}"; do
  echo "  ✓ $d"
done
