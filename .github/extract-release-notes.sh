#!/usr/bin/env bash
#
# Monta as notas de release em português para uma tag, para publicação
# automática pelo workflow .github/workflows/release.yml.
#
# Uso:
#   extract-release-notes.sh <tag> <caminho-do-changelog> <caminho-do-rodape>
#
# Fonte do texto: exclusivamente .github/release-notes/<tag>.md.
#
# Esse arquivo é obrigatório para publicar — não há fallback para a
# seção correspondente do CHANGELOG.md. Os dois documentos têm público
# diferente e não podem ser confundidos: o CHANGELOG.md é o registro
# técnico (nomes de função, tipos, decisões de implementação) para quem
# mantém o projeto; .github/release-notes/<tag>.md é o texto para quem
# só usa o programa. Se o fallback existisse, o dia em que alguém
# esquecesse de escrever as notas ao usuário resultaria em publicar o
# texto técnico do changelog como nota de release — sem ninguém notar,
# porque "funcionou" tecnicamente. É melhor falhar aqui, antes de
# compilar, e apontar exatamente o arquivo que falta criar.
#
# O caminho do changelog é recebido e validado (tem que existir) porque
# é o argumento que o workflow já passa e mantém o contrato estável,
# mas o conteúdo dele não é lido por este script.
#
# Depois do texto da tag, o conteúdo do arquivo de rodapé é anexado.
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
NOTES_PATH="${SCRIPT_DIR}/release-notes/${TAG}.md"

# Remove linhas em branco do início e do fim de stdin, preservando as
# do meio. Evita que o texto comece/termine com espaço em branco
# supérfluo antes de anexar o rodapé.
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

if [[ ! -f "$NOTES_PATH" ]]; then
  cat >&2 <<EOF
erro: notas de release ausentes para a tag '$TAG'.

Esperado: '$NOTES_PATH'

Esse arquivo é obrigatório e não tem substituto automático: a seção
correspondente do CHANGELOG.md é o registro técnico do projeto (nomes de
função, tipos, decisões de implementação) e não deve ser publicada como
nota de release. Crie '$NOTES_PATH' com o texto em português voltado a
quem usa o programa (o que mudou, por que importa, sem jargão de
implementação) — ver .github/release-notes/README.md — e mescle o
arquivo em main antes de publicar a tag.
EOF
  exit 1
fi

BODY="$(trim_blank_lines < "$NOTES_PATH")"

if [[ -z "$BODY" ]]; then
  echo "erro: '$NOTES_PATH' existe mas está vazio." >&2
  exit 1
fi

printf '%s\n' "$BODY"
printf '\n'
cat "$FOOTER_PATH"
