// Package commanddocs documenta, no mesmo formato usado pelas ferramentas
// (tool.Doc), os comandos do file-manager que NÃO são ferramentas do
// registry: undo, profiles (e seus quatro subcomandos), update, version e
// docs export.
//
// Por que um pacote à parte, em vez de uma função em internal/app (a
// sugestão original)? internal/app importa internal/ui/mainmenu (para abrir
// o menu interativo em NewRootCommand), e internal/ui/mainmenu precisa
// desta mesma lista para repassá-la a internal/ui/docs.NewScreen quando o
// usuário escolhe "Documentação" no menu — colocar CommandDocs() em
// internal/app criaria um ciclo de import (app -> mainmenu -> app). Este
// pacote não importa nem internal/app nem internal/ui/mainmenu (só
// internal/tool, que já é a base de todo o projeto), então tanto
// internal/app/root.go (para "file-manager docs export") quanto
// internal/ui/mainmenu (para a tela interativa) podem importá-lo
// livremente.
//
// CADA ENTRADA AQUI PRECISA CONTINUAR EXISTINDO DE VERDADE: o teste
// internal/app/command_docs_test.go (TestRootCommandsAreAllDocumented e
// TestCommandDocsFlagsMatchCobra) verifica, no comando raiz de verdade
// (NewRootCommand(...).Commands()), que todo subcomando que não é uma
// ferramenta do registry aparece aqui, e que as flags declaradas abaixo
// batem exatamente com as flags reais do cobra — nos dois sentidos.
package commanddocs

import "github.com/SamuelGFDias/file-manager/internal/tool"

// CommandDocs devolve a documentação exportável de cada comando auxiliar do
// file-manager (todo subcomando do comando raiz que não é uma ferramenta
// registrada em app.Tools()). Um comando pai que só agrupa subcomandos sem
// ter flags próprias (profiles, docs) NÃO aparece aqui como uma entrada
// única — cada subcomando FOLHA (list, export, import, path; export) tem a
// sua própria entrada, com o ID sendo o caminho completo digitado pelo
// usuário (ex: "profiles export"), porque é nesse nível que as flags reais
// existem.
func CommandDocs() []tool.Doc {
	return []tool.Doc{
		undoDoc(),
		profilesListDoc(),
		profilesExportDoc(),
		profilesImportDoc(),
		profilesPathDoc(),
		updateDoc(),
		versionDoc(),
		docsExportDoc(),
	}
}

func undoDoc() tool.Doc {
	return tool.Doc{
		ID:      "undo",
		Title:   "Desfazer (undo)",
		Summary: "Desfaz uma operação de organização registrada (hoje, só organize-pdf), devolvendo os arquivos ao estado anterior sempre que for seguro fazê-lo.",
		Description: "undo reverte uma operação registrada no histórico: para uma operação de CÓPIA " +
			"(o padrão de organize-pdf), apaga os arquivos que foram criados no destino — o arquivo " +
			"original em --input nunca é tocado, porque uma cópia nunca o modificou. Para uma operação " +
			"de MOVER (organize-pdf --move), devolve os arquivos à pasta de origem registrada no " +
			"manifesto. Em qualquer um dos dois casos, undo nunca toca em nada fora do que foi " +
			"registrado na hora da operação original: um arquivo cujo tamanho mudou desde então é " +
			"pulado (preservado, nunca apagado nem sobrescrito), e um arquivo cuja origem já esteja " +
			"ocupada por outro arquivo também é pulado, em vez de sobrescrever. Só existe histórico " +
			"para execuções reais — uma simulação (organize-pdf --dry-run) nunca gera manifesto, " +
			"então não há nada para \"desfazer\" a partir dela. --id escolhe a operação pelo " +
			"identificador exato (ver --list); --last escolhe a mais recente; sem nenhum dos dois, " +
			"e só em terminal interativo, um seletor pergunta qual operação desfazer. --dry-run mostra " +
			"o que seria feito sem tocar em nada. --list lista as operações registradas (--all remove " +
			"o limite padrão de exibição). --prune remove do disco os manifestos expirados (30 dias " +
			"para já desfeitas, 180 para pendentes, por padrão; --older-than substitui os dois " +
			"limiares por um único número de dias). --force permite tentar desfazer de novo uma " +
			"operação já marcada como desfeita.",
		WhenToUse: []string{
			"quando o usuário pedir para desfazer, reverter, cancelar ou \"voltar atrás\" numa organização/renomeação feita com organize-pdf",
			"quando o usuário quiser ver quais operações foram registradas antes de decidir qual desfazer",
			"quando o usuário quiser limpar manifestos de histórico antigos (poda), sem necessariamente desfazer nada",
		},
		Flags: []tool.FlagDoc{
			{Name: "id", Type: "string", Default: "", Description: "ID da operação a desfazer (ver \"file-manager undo --list\").", Example: "20260810-153000-ab12cd"},
			{Name: "last", Type: "bool", Default: "", Description: "Desfaz a operação registrada mais recente."},
			{Name: "list", Type: "bool", Default: "", Description: "Lista as operações registradas e sai."},
			{Name: "all", Type: "bool", Default: "", Description: "Com --list, mostra todas as operações, sem o limite padrão de exibição."},
			{Name: "dry-run", Type: "bool", Default: "", Description: "Só mostra o que seria feito, sem tocar em nada."},
			{Name: "yes", Shorthand: "y", Type: "bool", Default: "", Description: "Desfaz sem pedir confirmação."},
			{Name: "prune", Type: "bool", Default: "", Description: "Remove do disco os manifestos de histórico expirados e sai (não desfaz nada)."},
			{Name: "older-than", Type: "int", Default: "", Description: "Com --prune, usa N dias como limiar em vez do padrão (30 dias para já desfeitas, 180 para pendentes).", Example: "90"},
			{Name: "force", Type: "bool", Default: "", Description: "Permite desfazer uma operação que já foi desfeita antes."},
		},
		Examples: []tool.ExampleDoc{
			{Title: "Ver as operações registradas antes de decidir o que desfazer", Command: "file-manager undo --list"},
			{Title: "Desfazer a operação mais recente, com confirmação interativa", Command: "file-manager undo --last"},
			{Title: "Desfazer uma operação específica pelo ID, sem perguntar confirmação", Command: "file-manager undo --id 20260810-153000-ab12cd --yes"},
			{Title: "Simular o que seria desfeito por uma operação específica, sem alterar nada", Command: "file-manager undo --id 20260810-153000-ab12cd --dry-run"},
			{Title: "Remover do histórico manifestos com mais de 90 dias, perguntando confirmação", Command: "file-manager undo --prune --older-than 90"},
		},
		Notes: []string{
			"Operação de CÓPIA: desfazer apaga os arquivos criados no destino; o original em --input nunca é tocado.",
			"Operação de MOVER: desfazer devolve os arquivos à pasta de origem registrada.",
			"Nada fora do que foi registrado no manifesto da operação original é tocado.",
			"Um arquivo cujo tamanho mudou desde a operação original é preservado (pulado), nunca sobrescrito.",
			"Se a pasta de origem já tiver um arquivo com o mesmo nome, esse arquivo é pulado em vez de sobrescrito.",
			"Uma simulação (organize-pdf --dry-run) nunca gera histórico; não há o que desfazer a partir dela.",
		},
	}
}

func profilesListDoc() tool.Doc {
	return tool.Doc{
		ID:      "profiles list",
		Title:   "Perfis — listar (profiles list)",
		Summary: "Lista os perfis salvos, de uma ferramenta específica ou de todas as que suportam perfis.",
		Description: "profiles list mostra os perfis salvos localmente, agrupados por ferramenta. Sem " +
			"--tool, lista os perfis de todas as ferramentas que suportam perfis salvos; com --tool, " +
			"restringe a uma única ferramenta pelo ID (ex: organize-pdf). Faz parte da família de " +
			"subcomandos \"profiles\" (list, export, import, path), criada para o fluxo \"calibrar as " +
			"opções numa máquina e usar o mesmo perfil em outra\": depois de acertar flags complexas " +
			"(ex: as regex de organize-pdf) e salvar um perfil, profiles export empacota esse perfil " +
			"num único arquivo, profiles import o recria em outra máquina (ou para outra pessoa), e " +
			"profiles list/profiles path ajudam a inspecionar o que já está salvo.",
		WhenToUse: []string{
			"quando o usuário quiser saber quais perfis já salvou, de uma ferramenta específica ou de todas",
		},
		Flags: []tool.FlagDoc{
			{Name: "tool", Type: "string", Default: "", Description: "Filtra por ID da ferramenta (ex: organize-pdf).", Example: "organize-pdf"},
		},
		Examples: []tool.ExampleDoc{
			{Title: "Listar os perfis salvos de todas as ferramentas", Command: "file-manager profiles list"},
			{Title: "Listar só os perfis salvos de organize-pdf", Command: "file-manager profiles list --tool organize-pdf"},
		},
	}
}

func profilesExportDoc() tool.Doc {
	return tool.Doc{
		ID:      "profiles export",
		Title:   "Perfis — exportar (profiles export)",
		Summary: "Exporta um perfil salvo para um arquivo, para levar a outra máquina ou compartilhar com outra pessoa.",
		Description: "profiles export grava o perfil identificado por --tool e --name num arquivo YAML " +
			"autocontido em --output. É a metade de \"escrita\" do fluxo \"calibrar numa máquina, usar " +
			"em outra\": o arquivo gerado é o mesmo formato que profiles import espera de volta. As " +
			"três flags são obrigatórias.",
		WhenToUse: []string{
			"quando o usuário quiser levar um perfil já calibrado para outra máquina, ou enviá-lo a outra pessoa",
		},
		Flags: []tool.FlagDoc{
			{Name: "tool", Type: "string", Default: "", Description: "ID da ferramenta dona do perfil (obrigatória).", Example: "organize-pdf"},
			{Name: "name", Type: "string", Default: "", Description: "Nome do perfil a exportar (obrigatória).", Example: "padrao"},
			{Name: "output", Type: "string", Default: "", Description: "Caminho do arquivo de saída (obrigatória).", Example: "./perfil-padrao.yaml"},
		},
		Examples: []tool.ExampleDoc{
			{Title: "Exportar o perfil \"padrao\" de organize-pdf para um arquivo", Command: "file-manager profiles export --tool organize-pdf --name padrao --output ./perfil-padrao.yaml"},
			{Title: "Exportar um perfil de merge-pdf para enviar a outra pessoa", Command: "file-manager profiles export --tool merge-pdf --name mensal --output ./perfil-mensal.yaml"},
		},
	}
}

func profilesImportDoc() tool.Doc {
	return tool.Doc{
		ID:      "profiles import",
		Title:   "Perfis — importar (profiles import)",
		Summary: "Importa um perfil de um arquivo gerado por profiles export (ou recebido de outra pessoa).",
		Description: "profiles import lê o arquivo em --file, confirma que a ferramenta que ele " +
			"referencia existe neste CLI e suporta perfis, e decodifica o conteúdo na struct de opções " +
			"dessa ferramenta — um arquivo corrompido ou de versão incompatível falha aqui, na " +
			"importação, em vez de falhar mais tarde ao tentar usar o perfil. Por padrão usa o nome já " +
			"gravado no arquivo; --name sobrescreve esse nome no destino. --force é necessário para " +
			"sobrescrever um perfil já existente com o mesmo nome nesta máquina; sem --force, a " +
			"importação para um nome já ocupado falha.",
		WhenToUse: []string{
			"quando o usuário receber um arquivo de perfil (de profiles export ou de outra pessoa) e quiser usá-lo nesta máquina",
		},
		Flags: []tool.FlagDoc{
			{Name: "file", Type: "string", Default: "", Description: "Caminho do arquivo de perfil a importar (obrigatória).", Example: "./perfil-padrao.yaml"},
			{Name: "name", Type: "string", Default: "", Description: "Sobrescreve o nome do perfil importado.", Example: "padrao-equipe"},
			{Name: "force", Type: "bool", Default: "", Description: "Sobrescreve um perfil existente com o mesmo nome."},
		},
		Examples: []tool.ExampleDoc{
			{Title: "Importar um perfil recebido, mantendo o nome original", Command: "file-manager profiles import --file ./perfil-padrao.yaml"},
			{Title: "Importar um perfil recebido com um nome novo, sobrescrevendo se já existir", Command: "file-manager profiles import --file ./perfil-padrao.yaml --name padrao-equipe --force"},
		},
	}
}

func profilesPathDoc() tool.Doc {
	return tool.Doc{
		ID:      "profiles path",
		Title:   "Perfis — caminho (profiles path)",
		Summary: "Mostra o diretório onde os perfis salvos ficam guardados no disco.",
		Description: "profiles path imprime o caminho absoluto da pasta de perfis (ex: " +
			"~/.config/file-manager/profiles no Linux, %AppData%\\file-manager\\profiles no Windows), " +
			"para quem quiser localizar, copiar ou inspecionar os arquivos manualmente sem saber de " +
			"cor a convenção de diretório de configuração do sistema operacional. Não recebe flags.",
		WhenToUse: []string{
			"quando o usuário quiser saber onde os perfis ficam salvos, para inspecionar ou copiar os arquivos manualmente",
		},
		Examples: []tool.ExampleDoc{
			{Title: "Mostrar o diretório onde os perfis ficam salvos", Command: "file-manager profiles path"},
			{Title: "Listar os arquivos de perfil dentro do diretório (Linux)", Command: `ls "$(file-manager profiles path)"`},
		},
	}
}

func updateDoc() tool.Doc {
	return tool.Doc{
		ID:      "update",
		Title:   "Atualizar (update)",
		Summary: "Consulta o último release publicado no GitHub e, quando autorizado, baixa e substitui o próprio binário.",
		Description: "update é o único caminho de atualização do file-manager: consulta os releases " +
			"publicados no repositório oficial, compara com a versão em execução e classifica a " +
			"diferença em três severidades — correção de defeito (patch), novidade sem quebra (minor) " +
			"ou mudança incompatível (major) — para que o usuário saiba se é seguro atualizar sem " +
			"revisar as notas do release. Sem --yes, pede confirmação antes de baixar e substituir o " +
			"executável (a mensagem de confirmação já cita a severidade quando é patch ou major). Com " +
			"--check, só mostra se há versão nova, sem baixar nem substituir nada. Uma build local " +
			"(versão não-semver, ex: \"dev\") não é comparada; update sempre relata a versão mais " +
			"recente publicada nesse caso.",
		WhenToUse: []string{
			"quando o usuário pedir para atualizar, verificar por atualização, ou saber se está usando a versão mais recente do file-manager",
		},
		Flags: []tool.FlagDoc{
			{Name: "yes", Shorthand: "y", Type: "bool", Default: "", Description: "Atualiza sem pedir confirmação."},
			{Name: "check", Type: "bool", Default: "", Description: "Só verifica se há versão nova, sem baixar nem substituir."},
		},
		Examples: []tool.ExampleDoc{
			{Title: "Verificar se há uma versão nova, sem baixar nada", Command: "file-manager update --check"},
			{Title: "Atualizar para a versão mais recente, confirmando manualmente", Command: "file-manager update"},
			{Title: "Atualizar sem pedir confirmação (ex: dentro de um script)", Command: "file-manager update --yes"},
		},
		Notes: []string{
			"O aviso de atualização distingue correção de defeito (patch) de novidade sem quebra (minor) e de mudança incompatível (major); patch e major recebem destaque visual maior.",
			"Uma build local (versão não-semver, ex: \"dev\") não é comparada contra os releases publicados; update sempre relata o release mais recente nesse caso.",
		},
	}
}

func versionDoc() tool.Doc {
	return tool.Doc{
		ID:      "version",
		Title:   "Versão (version)",
		Summary: "Mostra a versão do binário em execução (versão, commit e data de build).",
		Description: "version imprime a versão semântica do binário junto do hash curto do commit e " +
			"da data/hora de build, no formato \"<versão> (<commit>, <data>)\". A flag global " +
			"\"--version\" (e o atalho \"-v\"), aceita no comando raiz, produzem exatamente a mesma " +
			"saída — as duas formas convivem, nenhuma é \"a de verdade\": quem digita \"--version\" " +
			"por reflexo e quem prefere o subcomando são atendidos igualmente.",
		WhenToUse: []string{
			"quando o usuário perguntar qual versão do file-manager está instalada, ou pedir para conferir a versão antes de reportar um problema",
		},
		Examples: []tool.ExampleDoc{
			{Title: "Mostrar a versão do binário", Command: "file-manager version"},
			{Title: "Mostrar a versão usando o atalho --version", Command: "file-manager --version"},
		},
		Notes: []string{
			"\"file-manager --version\" e \"file-manager -v\" produzem exatamente a mesma saída que \"file-manager version\".",
		},
	}
}

func docsExportDoc() tool.Doc {
	return tool.Doc{
		ID:      "docs export",
		Title:   "Documentação (docs export)",
		Summary: "Exporta a documentação exportável do próprio CLI — as ferramentas e os comandos auxiliares — para um arquivo Markdown.",
		Description: "docs export é o comando que gera os dois arquivos que este próprio documento " +
			"representa: --format context produz um documento rico para colar numa conversa pontual " +
			"com uma IA, e --format skill produz um arquivo no formato de skill de agente (frontmatter " +
			"YAML + corpo Markdown), pensado para instalação persistente. Os dois são gerados a partir " +
			"da mesma fonte de dados usada pelo binário real (a Doc() de cada ferramenta, mais a " +
			"documentação dos comandos auxiliares), então nunca divergem do comportamento efetivamente " +
			"implementado na versão indicada no cabeçalho do arquivo gerado. Documentar esta própria " +
			"ferramenta evita que uma IA lendo o resultado precise adivinhar como o usuário gerou o " +
			"arquivo que ela está lendo.",
		WhenToUse: []string{
			"quando o usuário pedir para gerar, exportar ou atualizar a documentação do file-manager para uma IA",
			"quando o usuário quiser instalar ou reinstalar o SKILL.md deste CLI num agente de IA",
		},
		Flags: []tool.FlagDoc{
			{Name: "format", Shorthand: "f", Type: "string", Default: "context", Description: "Formato da documentação (\"context\" ou \"skill\")."},
			{Name: "output", Shorthand: "o", Type: "string", Default: "", Description: "Caminho do arquivo de saída (obrigatória).", Example: "./SKILL.md"},
		},
		Examples: []tool.ExampleDoc{
			{Title: "Gerar a documentação de contexto completa, para colar numa conversa", Command: "file-manager docs export --format context --output ./file-manager-docs.md"},
			{Title: "Gerar o SKILL.md para instalar num agente de IA persistente", Command: "file-manager docs export --format skill --output ./SKILL.md"},
		},
	}
}
