#!/usr/bin/env bash
#
# Monta as notas de release em português para uma tag, para publicação
# automática pelo workflow .github/workflows/release.yml.
#
# Uso:
#   extract-release-notes.sh <tag> <caminho-do-changelog> <caminho-do-rodape>
#
# Fonte do texto, em ordem de prioridade:
#   1. .github/release-notes/<tag>.md, se existir — texto próprio, mais
#      livre que uma entrada de changelog (ver .github/release-notes/README.md).
#   2. A seção correspondente do changelog: "## [X.Y.Z]", com ou sem
#      data ao lado (ex.: "## [0.12.0]" ou "## [0.12.0] - 2026-08-11").
#
# Em qualquer um dos dois casos, o conteúdo do arquivo de rodapé é
# anexado ao final. Se nenhuma das duas fontes tiver conteúdo para a
# tag, o script falha (código de saída != 0) com uma mensagem em stderr
# citando a tag — é melhor falhar aqui, antes de compilar e publicar,
# do que publicar um release sem notas.
set -euo pipefail

usage() {
  echo "Uso: $(basename "$0") <tag> <caminho-do-changelog> <caminho-do-rodape>" >&2
}

if [[ $# -ne 3 ]]; then
  usage
  exit 1
fi

TAG="$1"
CHANGELOG_PATH="$2"
FOOTER_PATH="$3"

if [[ -z "$TAG" ]]; then
  echo "erro: tag vazia" >&2
  exit 1
fi

if [[ ! -f "$CHANGELOG_PATH" ]]; then
  echo "erro: changelog não encontrado em '$CHANGELOG_PATH'" >&2
  exit 1
fi

if [[ ! -f "$FOOTER_PATH" ]]; then
  echo "erro: rodapé não encontrado em '$FOOTER_PATH'" >&2
  exit 1
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RICH_NOTES_PATH="${SCRIPT_DIR}/release-notes/${TAG}.md"

# Versão sem o prefixo "v" (ex.: v0.12.0 -> 0.12.0), usada para casar
# com o cabeçalho "## [0.12.0]" do changelog. Como é comparada de forma
# literal (com pontos escapados) contra a versão da tag, nunca casa por
# acidente com "## [Não publicado]", que não tem formato de versão.
VERSION="${TAG#v}"

# Remove linhas em branco do início e do fim de stdin, preservando as
# do meio. Evita que a seção extraída comece/termine com espaço em
# branco supérfluo antes de anexar o rodapé.
trim_blank_lines() {
  awk '
    { lines[NR] = $0 }
    END {
      start = 1
      end = NR
      while (start <= end && lines[start] ~ /^[ \t]*$/) start++
      while (end >= start && lines[end] ~ /^[ \t]*$/) end--
      for (i = start; i <= end; i++) print lines[i]
    }
  '
}

# Imprime o conteúdo da seção "## [<versão>]" do changelog (sem incluir
# o próprio cabeçalho nem a seção seguinte). Sai com código != 0, sem
# imprimir nada, se a seção não existir.
extract_from_changelog() {
  awk -v ver="$VERSION" '
    BEGIN {
      gver = ver
      gsub(/\./, "\\.", gver)
      header_re = "^## \\[" gver "\\]([ \t]*-.*)?[ \t]*$"
      in_section = 0
      found = 0
    }
    {
      if ($0 ~ header_re) {
        in_section = 1
        found = 1
        next
      }
      if (in_section && $0 ~ /^## \[/) {
        in_section = 0
      }
      if (in_section) {
        print
      }
    }
    END {
      exit(found ? 0 : 1)
    }
  ' "$CHANGELOG_PATH"
}

BODY=""

if [[ -f "$RICH_NOTES_PATH" ]]; then
  BODY="$(trim_blank_lines < "$RICH_NOTES_PATH")"
else
  RAW=""
  if ! RAW="$(extract_from_changelog)"; then
    echo "erro: nenhuma seção '## [${VERSION}]' encontrada em '$CHANGELOG_PATH' para a tag '$TAG', e não existe '$RICH_NOTES_PATH'." >&2
    exit 1
  fi
  BODY="$(printf '%s\n' "$RAW" | trim_blank_lines)"
fi

if [[ -z "$BODY" ]]; then
  echo "erro: a seção de notas encontrada para a tag '$TAG' está vazia." >&2
  exit 1
fi

printf '%s\n' "$BODY"
printf '\n'
cat "$FOOTER_PATH"
