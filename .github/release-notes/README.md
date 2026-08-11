# Notas de release próprias

Por padrão, o workflow de release (`.github/workflows/release.yml`) usa como
notas do release a seção correspondente do `CHANGELOG.md` (ex.: a tag
`v0.12.0` usa a seção `## [0.12.0]`), via
`.github/extract-release-notes.sh`.

Quando o texto do `CHANGELOG.md` não tem o tom certo para quem só usa o
programa — ele é escrito para quem acompanha o desenvolvimento, e às vezes
uma versão merece uma explicação mais longa, com contexto de "por quê" e
exemplos de uso — crie um arquivo `<tag>.md` aqui (ex.: `v0.12.0.md`) com o
texto final em português. Quando esse arquivo existe, o script o usa **no
lugar** da seção do changelog (o rodapé de `.github/release-footer.md`
continua sendo anexado do mesmo jeito).

Esse arquivo deve ser escrito e revisado **no mesmo PR** que introduz a
mudança — é essa a mudança de fundo deste mecanismo: o texto voltado ao
usuário final passa a ser revisado junto com a feature, não redigido depois
que o release já foi publicado.

Se não existir `<tag>.md` para a tag sendo publicada, o script cai
automaticamente na seção do `CHANGELOG.md` — não é obrigatório criar um
arquivo aqui para toda versão, só quando o changelog não for suficiente.
