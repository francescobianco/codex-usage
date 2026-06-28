# codex-usage

CLI in Go per ispezionare lo stato di utilizzo di GPT Codex leggendo i log reali sotto `~/.codex/sessions`.

Mostra:

- sessione corrente o piu recente
- stato del turn corrente
- token totali e ultimo turn
- finestra di contesto e percentuale usata
- rate limits osservati
- contatori attivita, tool call e metadati runtime

## Uso

```bash
go run ./cmd/codex-usage
go run ./cmd/codex-usage --json
go run ./cmd/codex-usage --session 019f1068-59f4-70d1-b34d-0ba421052720
go run ./cmd/codex-usage --codex-home /path/to/.codex
```

## Reconcile (locale vs server)

Confronta TUTTE le sessioni locali con il contatore weekly riportato dal server,
per capire se la quota addebitata e' spiegata dalle sessioni di questo PC.

```bash
go run ./cmd/codex-usage --reconcile
go run ./cmd/codex-usage --reconcile --json
```

Per ogni finestra weekly mostra:

- `usato` / `rimane` (dal server)
- `spiegato da sessioni locali` vs `NON spiegato` (altro PC, web/cloud, o accesso non autorizzato)
- per ogni sessione: prompt, delta% weekly, token committati e token **rolled-back**
  (turn abortiti: pagati al server ma non contati localmente)
- avvisi sui consumi senza alcuna sessione locale nella finestra

## Probe (sonde live)

Pilota `codex exec` con prompt-sonda e misura quanto si muovono i contatori 5h e
weekly per ciascun turno. **Ogni sonda consuma quota reale.**

```bash
go run ./cmd/codex-usage --probe
go run ./cmd/codex-usage --probe --probe-prompt "ciao" --probe-count 3
go run ./cmd/codex-usage --probe --probe-model gpt-5.4 --json
```
