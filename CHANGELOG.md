# Changelog

Todas as mudanças notáveis neste projeto são documentadas neste arquivo.

O formato é baseado em [Keep a Changelog](https://keepachangelog.com/pt-BR/1.0.0/), e este projeto segue [Versionamento Semântico](https://semver.org/lang/pt-BR/).

## Política de Versionamento

- **MAJOR:** Incrementado quando há mudanças incompatíveis nas flags de linha de comando (renomeação, remoção ou mudança semântica de uma flag existente) ou no formato de arquivo YAML de perfis. Quebra automações de quem já usa a ferramenta.
- **MINOR:** Incrementado quando são adicionadas novas ferramentas, novos modos, ou novas flags que não quebram compatibilidade com versões anteriores.
- **PATCH:** Incrementado para correções de bugs e melhorias internas que não afetam a interface externa.

## [Não publicado]

### Planejado

- [ ] Suporte para criptografia de perfis
- [ ] Modo batch para processing em lote via arquivo JSON de configuração
- [ ] Temas customizáveis para a interface interativa

## [0.3.1] - 2026-08-11

### Corrigido

- **Aviso de versão nova no menu principal agora aparece na primeira abertura.** A verificação roda em segundo plano e leva cerca de 250ms, mas o menu consultava o resultado uma única vez, no instante da abertura, quando ainda não havia chegado — e o seletor assume o terminal logo em seguida, sem redesenhar o menu. Na prática o aviso só apareceria se o usuário entrasse numa ferramenta e voltasse. `selfupdate.Checker` ganhou `WaitNotice(timeout)`, que aguarda no máximo o timeout pelo resultado; o menu usa `WaitNotice(1,5s)` só na primeira renderização e `Notice()` (não bloqueante) nas seguintes. Sem internet ou se o tempo estourar, o menu abre normalmente e nada é exibido.

## [0.3.0] - 2026-08-11

### Adicionado

- **Auto-atualização: novo subcomando `update`.** Consulta o último release publicado no GitHub, compara com a versão em execução e, se houver uma mais nova, mostra a progressão (ex: `v0.1.0 → v0.2.1`) e o link do release antes de pedir confirmação. `-y`/`--yes` atualiza sem perguntar; `--check` só verifica, sem baixar nem substituir nada. Já na última versão, informa e sai sem erro. Build local (versão `dev`) é detectado e avisado. Antes de substituir o executável em uso, o binário baixado é executado para validação — um download corrompido aborta a troca sem tocar no executável atual. Novo pacote `internal/selfupdate`, sem dependências externas (só biblioteca padrão do Go).
- **Aviso de versão nova no menu principal.** Quando há uma versão mais recente publicada, o menu exibe um aviso com a progressão de versão e o comando para atualizar. A verificação roda em segundo plano, uma única vez por sessão, e nunca bloqueia a abertura do menu. Sem internet, o menu abre normalmente e nenhum aviso aparece — falha de rede é silenciosa, nunca um erro na tela. Build local (`dev`) não gera aviso.

### Modificado

- **Interface mais legível.** No menu principal, a descrição de cada ferramenta agora aparece só na opção destacada (acompanhando a seta), em vez de todas ao mesmo tempo — feito sobrescrevendo o template de seleção do `survey`. A dica `[Use arrows to move, type to filter]`, única parte da interface em inglês, foi traduzida para `[use ↑ ↓ para navegar, digite para filtrar, Enter para confirmar]`. Novos helpers visuais em `internal/ui` (`Bold`, `Highlight`, `PathText`, `Count`, `Step`, `Divider`, `Blank`). O fluxo de `organize-pdf` passou a mostrar "Passo N de 7" em cada etapa, com separação entre blocos e destaque em caminhos e quantidades — sem alterar nenhum texto de pergunta nem a ordem das etapas.

## [0.2.1] - 2026-08-11

### Corrigido

- **`organize-pdf`: pasta de origem sem PDFs agora é avisada na hora.** Antes, o usuário só descobria a pasta errada no final, depois de toda a calibração de regex, com um "0 de 0 arquivos" sem explicação. Agora a contagem de PDFs acontece no ato da seleção da pasta de origem: zero PDFs bloqueia o avanço e oferece escolher outra pasta (limite de 5 tentativas); com PDFs, confirma de imediato quantos foram encontrados.
- **`organize-pdf`: aviso quando o PDF de amostra está fora da pasta de origem**, com confirmação explícita (default: não) antes de seguir em frente — foi assim que o usuário calibrou as regras contra um documento que não fazia parte do lote a processar.
- **`organize-pdf`: resumo antes de aplicar**, com caminhos absolutos de origem e destino, contagem de PDFs, e se a operação vai copiar ou mover.
- **`organize-pdf`: prompts de seleção agora identificam a etapa** (`PASTA DE ORIGEM`, `PASTA DE DESTINO`, `PDF de AMOSTRA`) em vez do genérico "Selecione um diretório" nas duas etapas.
- **Seletores de pasta encadeados não voltam mais ao diretório do executável.** Depois de escolher a pasta de origem, o prompt de destino reabria em `~/.file_manager` (pasta do binário), que não tem subpastas e por isso parecia vazia — o usuário concluía que a seleção não tinha funcionado e refazia toda a navegação. O prompt de destino agora começa na pasta de origem recém-selecionada, e o `filepicker` ganhou memória do último diretório usado (`LastDir()`/`resolveStart()`), respeitada sempre que o chamador não passa um `start` explícito.

### Notas

- Descoberta relacionada, documentada em `AGENTS.md` e no `README.md`: em Go, `.` na regex não casa quebra de linha, e texto de OCR quebra linha com frequência — regex que precisam atravessar linhas exigem o prefixo `(?s)`.

## [0.2.0] - 2026-08-11

### Adicionado

- **Suporte a OCR** para PDFs digitalizados (sem camada de texto): quando `split-pdf --mode regex` ou `organize-pdf --level` encontram uma página sem texto embutido, a imagem da página é extraída pelo pdfcpu e lida via [Tesseract](https://github.com/tesseract-ocr/tesseract) (executável externo, invocado por `os/exec` — não é binding CGO, então o binário continua estático e cross-compilável para Windows).
- **Flags `--ocr` e `--ocr-lang`** em `split-pdf` e `organize-pdf`: `--ocr` controla o modo (`auto`, `always` ou `never`; default `auto`) e `--ocr-lang` o idioma do OCR (default `por`). Persistidas nos perfis YAML como `ocr` e `ocr_lang`.
- **Novo pacote `internal/ocr`:** wrapper do executável `tesseract`, com detecção automática (variável `TESSERACT_PATH`, PATH e, no Windows, os caminhos usuais de instalação) e aviso de instalação por sistema operacional quando ausente.
- Em `organize-pdf`, a calibração interativa passa a usar o mesmo `TextOptions` do processamento real, garantindo que a regex calibrada veja o mesmo texto (nativo ou de OCR) que a execução vai processar.

### Notas

- Sem o Tesseract instalado, nada quebra: a execução segue normalmente com um aviso, sem OCR.
- O OCR custa aproximadamente 1 segundo por página.
- O reconhecimento de caracteres não é perfeito (ex: `ESCOLA` pode virar `ESCO`, `0` pode ser confundido com `O`) — regex sobre conteúdo que pode ter passado por OCR devem ser tolerantes a esse tipo de erro.

## [0.1.0] - 2026-08-11

### Adicionado

- **Menu interativo:** Interface de navegação em pilha com seleção de ferramentas via cursor e teclado.
- **Ferramenta `merge-pdf`:** Une múltiplos PDFs com suporte a recursão em pastas, ordenação por nome ou data de modificação.
- **Ferramenta `split-pdf`:** Separa PDFs em três modos: página individual, intervalos definidos ou padrão regex no conteúdo (com captura para nomes).
- **Ferramenta `organize-pdf`:** Organiza PDFs em hierarquia de pastas com base em regras regex multi-nível; funciona também como renomeador em lote.
- **Sistema de perfis:** Salva e carrega configurações em YAML de forma persistente (plataforma: `%AppData%\file-manager\profiles` no Windows, `~/.config/file-manager/profiles` em Linux/macOS).
- **Exportação de documentação:** Dois formatos de saída para documentar a ferramenta em contextos de IA:
  - Formato `context`: Markdown detalhado para colar em chat.
  - Formato `skill`: YAML com frontmatter para instalar em agentes de IA.
- **Gerador de scaffold:** Comando `make new-tool NAME=...` para criar uma ferramenta nova com estrutura de arquivos pré-pronta.
- **Build cross-platform:** Makefile com alvos para compilação nativa (Linux, Windows) com `CGO_ENABLED=0` garantindo distribuição como binário único sem dependências de runtime.
- **Testes:** Cobertura de testes para lógica de domínio (merge, split, organize, calibração de regex) com abordagem de testes de tabela.
- **Linting e formatação:** Targets `make lint` e `make fmt` com suporte opcional a golangci-lint.

### Removido

- Nenhuma mudança em versão 0.1.0 (lançamento inicial).

---

**Nota de compatibilidade:** Esta é a versão inicial. Recomenda-se que usuários dependam de perfis (salvar via interface ou arquivos YAML) para suas automações, garantindo portabilidade entre versões MINOR e PATCH futuras.
