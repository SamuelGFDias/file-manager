Este arquivo é um modelo de referência — não corresponde a nenhuma tag
real e o workflow de release nunca o lê (só arquivos nomeados `vX.Y.Z.md`
são usados). Copie a estrutura abaixo ao criar as notas de uma versão de
verdade.

---

**Um registro de histórico corrompido não impede mais o `undo --list` de
mostrar as outras operações.** Antes, um único arquivo danificado no meio
do histórico (por exemplo, por falta de espaço em disco durante uma
gravação anterior) bloqueava a listagem inteira — inclusive o "desfazer"
de tudo o mais que ainda estava íntegro, exatamente quando esse recurso
mais faz falta. Agora o `file-manager` avisa sobre o arquivo com problema
e continua mostrando o restante normalmente.

**`undo --list` agora mostra só as 20 operações mais recentes por
padrão**, com um aviso no rodapé quando há mais (`use --all para ver
todos`). Quem quiser a lista completa pode rodar `undo --all`.

**Novo comando `undo --prune`** remove manualmente registros de
histórico antigos (operações já desfeitas há mais de 30 dias, ou nunca
desfeitas há mais de 180 dias), com confirmação antes de apagar.

---

Note o contraste com a entrada equivalente no `CHANGELOG.md`, que cita a
mudança de tipo de retorno de `internal/history.List` (`[]Manifest` →
`[]Header`), o motivo de memória (60 mil entradas evitadas), e o nome dos
símbolos internos envolvidos. Nada disso aparece aqui — quem lê este
texto não roda `go build`.
