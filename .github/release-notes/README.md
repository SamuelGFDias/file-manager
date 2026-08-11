# Notas de release

Criar `<tag>.md` aqui (ex.: `v0.12.0.md`) **é parte de lançar uma versão**,
não um extra opcional. `.github/extract-release-notes.sh` exige esse
arquivo: sem ele, o workflow de release falha antes de compilar.

## Por que não usar o CHANGELOG.md diretamente

`CHANGELOG.md` e `.github/release-notes/<tag>.md` têm público diferente e
**não compartilham texto**:

- **`CHANGELOG.md`** é o registro técnico do projeto, para quem mantém o
  código: cita nomes de função, tipos Go, pacotes, decisões de
  implementação. Continua sendo preenchido normalmente a cada mudança
  (ver `docs/CONTRIBUTING.md`).
- **`.github/release-notes/<tag>.md`** é o texto publicado no release do
  GitHub, para quem só usa o programa — baixa o `.exe`, dá duplo clique,
  nunca leu uma linha de Go. Fala o que mudou e por que importa para
  quem usa, sem jargão de implementação.

Não existe fallback de um para o outro. Um fallback pareceria uma rede de
segurança, mas na prática garantiria que, no dia em que alguém esquecesse
de escrever as notas ao usuário, o release saísse com o texto técnico do
changelog — sem ninguém perceber, porque tecnicamente "funcionou". Falhar
é melhor: o release não sai errado, e a correção é só criar o arquivo.

## O que escrever

- O que mudou, na perspectiva de quem usa: uma ferramenta nova, um
  comportamento diferente, um bug que sumiu.
- Por que importa: o problema que isso resolve, ou o que o usuário ganha.
- Como usar, se for o caso (flag nova, exemplo de comando).

## O que não escrever

- Nomes de função, tipo, pacote ou arquivo (`internal/history.List`,
  `[]Header`, `ExtractPageTextsOpts`...).
- Detalhes de implementação (estrutura de dados escolhida, algoritmo,
  por que um teste foi reescrito).
- Contagens de memória, complexidade, ou qualquer coisa que só importa a
  quem lê o código-fonte.

Isso é o que o `CHANGELOG.md` já registra — não repita, traduza.

## Exemplo: a mesma mudança, escrita para cada público

**No `CHANGELOG.md`** (registro técnico):

> `internal/history.List` agora pula cada arquivo que falhar ao ler ou
> decodificar, reporta um aviso citando o nome do arquivo e o motivo, e
> continua listando os demais. `err == nil` nesse caso — só um problema
> no PRÓPRIO diretório de histórico ainda propaga erro.

**Em `.github/release-notes/<tag>.md`** (texto ao usuário):

> Um registro de histórico corrompido não impede mais o `undo --list` de
> mostrar as outras operações. Antes, um único arquivo danificado no meio
> do histórico (por exemplo, por falta de espaço em disco) bloqueava a
> listagem inteira — inclusive o "desfazer" de tudo o mais que ainda
> estava íntegro. Agora o `file-manager` avisa sobre o arquivo com
> problema e segue mostrando o restante normalmente.

Mesma mudança, dois textos. Nenhum é resumo do outro — cada um responde
a uma pergunta diferente ("o que mudou no código" vs. "o que muda para
mim").

## Como usar

1. Ao preparar o PR de uma mudança visível para o usuário, escreva a
   entrada técnica em `CHANGELOG.md` (seção `[Não publicado]`) **e**
   crie/edite `.github/release-notes/<tag-planejada>.md` com o texto ao
   usuário. Os dois são revisados juntos no mesmo PR.
2. Quando a versão for lançada (`git tag vX.Y.Z && git push origin
   vX.Y.Z`), o workflow usa esse arquivo como nota do release e anexa o
   rodapé fixo (`.github/release-footer.md`).
3. Se o arquivo não existir na hora de lançar, o workflow falha antes de
   compilar, com uma mensagem citando o caminho exato que falta criar.

Veja `.github/release-notes/EXEMPLO.md` para um modelo de estrutura (não
é usado por nenhuma tag real — é só referência).
