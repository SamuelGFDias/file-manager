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
