# Plano de continuação do Superpose

> Documento de planejamento — outubro/2026. Baseado na análise do código em `main` (commit `2ed4d15`).

## 1. Onde o projeto está hoje

O Superpose é um daemon em Go (~1.700 linhas, sem testes) que:

- lê `watchers.yml`, observa diretórios recursivamente com um wrapper próprio de `inotify` (só Linux);
- envia criações/alterações/remoções para uma pasta do Google Drive (Drive API v3), gravando o caminho absoluto em `appProperties`;
- mantém um cache `path → driveId` em SQLite.

O roadmap do README parou exatamente na metade difícil: **não há sincronização remoto → local**, nem detecção de conflitos, nem sync inicial. Ou seja, hoje é um *backup unidirecional*, não um sync.

### 1.1 Problemas encontrados no código

| # | Onde | Problema |
|---|------|----------|
| 1 | `main.go` | `app.Run()` só imprime "boom!" e `start()` roda depois de qualquer comando (inclusive `--help`). A CLI não tem comandos reais. |
| 2 | `ConfigFile.ParseFile` | Lê `Info.Name()` (só o *basename*), então só funciona se o processo rodar no diretório do arquivo. |
| 3 | `adapters/sqlite` | A tabela `worktree` **nunca é criada** (não há schema/migração). `Upsert` falha num banco novo. |
| 4 | `adapters/sqlite` | `Stmt.Close()` é no-op e cada query faz `Prepare` → vazamento de statements. `DebugEntry` usa receiver por valor, então o timing nunca é registrado. |
| 5 | `main.receiveEvents` | `eventPath.Mask += event.Mask` soma bits em vez de fazer OR — eventos repetidos corrompem a máscara. |
| 6 | `main.syncLocalToRemote` | Renomear = `IN_MOVED_FROM` apaga no remoto, mas `IN_MOVED_TO` é ignorado → **arquivo renomeado some do remoto** até ser editado de novo. |
| 7 | `inotify/watcher.go` | Em `IN_DELETE_SELF`, `event.Name` já é o próprio diretório; `filepath.Dir(...)` remove o watch do **pai**. |
| 8 | `inotify/watcher.go` | Mapa global `WatchingPaths` acessado por várias goroutines sem lock (data race). `IN_Q_OVERFLOW` não é tratado (perde eventos silenciosamente). Um erro em `AddWatcher` fecha o inotify inteiro. |
| 9 | `GoogleAPI/drive.go` | `os.Open` sem `Close` (vazamento de fd); `log.Fatal` em caminhos normais (ex.: `stat` de arquivo apagado entre o evento e o upload derruba o daemon); query do Drive montada por concatenação (`'` no nome quebra a query). |
| 10 | Modelo de dados | Usa **caminho absoluto local** como identidade remota (`fullPath`). Não funciona entre máquinas com `$HOME` diferente — e é impossível no celular. |
| 11 | Segurança | `client_secret` e tokens OAuth em texto puro no `watchers.yml`, que é reescrito pelo programa (perde comentários). O exemplo do README sincroniza `~/.ssh` **sem criptografia** para um provedor de nuvem. |
| 12 | Repo | `build/go_build_zero_sync` é um artefato binário de outro projeto (`zero-sync`, Go 1.18) commitado. `go vet` falha (`log.Fatalln(os.Stderr, …)`, struct literal sem chaves). Sem CI. |

**Conclusão:** vale mais reestruturar o núcleo do que remendar. O que se aproveita bem: a ideia de configuração por YAML, ignores, a lógica de walk recursivo e o aprendizado sobre eventos do inotify.

## 2. Atualização de bibliotecas e toolchain

| Hoje | Proposta | Motivo |
|------|----------|--------|
| `go 1.18` | `go 1.27` (toolchain atual: 1.27.1) | 1.18 está sem suporte há anos; ganha `log/slog`, `iter`, `min/max`, loop var por iteração, `testing/synctest`. |
| inotify próprio (`syscall`) | `github.com/fsnotify/fsnotify` v1.10 | Multiplataforma (Linux, macOS, Windows, BSD). Remove ~500 linhas de `unsafe`/syscall. Recursão continua manual, mas é simples. |
| `github.com/mattn/go-sqlite3` v1.14.14 (cgo) | `modernc.org/sqlite` v1.x (Go puro) | Sem cgo: cross-compile trivial e **funciona com gomobile no Android/iOS** (essencial para reaproveitar o núcleo no celular). |
| sem migrações | `github.com/pressly/goose/v3` (ou SQL embutido com `embed` + `PRAGMA user_version`) | Schema versionado; resolve o bug #3. |
| `github.com/urfave/cli/v2` v2.10 | `github.com/urfave/cli/v3` v3.14 (ou `spf13/cobra` v1.10) | v3 é a linha ativa, com `context.Context` nativo. |
| `gopkg.in/yaml.v3` | `go.yaml.in/yaml/v3` v3.0.5 | O `gopkg.in/yaml.v3` foi arquivado; `go.yaml.in` é o fork mantido pela organização YAML (API idêntica). |
| `log` + `ioutil` | `log/slog` + `os`/`io` | `ioutil` está obsoleto; logs estruturados com nível. |
| `golang.org/x/oauth2` 2022, `google.golang.org/api` v0.84 | `oauth2` v0.37, `api` v0.300 — **só se o adaptador Google Drive for mantido** | Ver seção 3. |
| — | `github.com/aws/aws-sdk-go-v2/service/s3` v1.114 | Upload/download de blobs (Supabase Storage, R2, MinIO, B2, S3 — todos falam S3). |
| — | `filippo.io/age` v1.3 **ou** `golang.org/x/crypto/chacha20poly1305` + `argon2` | Criptografia ponta a ponta. |
| — | `github.com/zalando/go-keyring` | Guardar refresh token e chave no keyring do SO, fora do YAML. |

Ferramentas: `golangci-lint` (com `govet`, `staticcheck`, `errcheck`), `go test -race`, GitHub Actions, `goreleaser` para binários.

## 3. Escolha do backend

O Google Drive não foi feito para isso: a Activity API tem latência e cotas, não há controle de concorrência (não dá para dizer "só grave se ainda estiver na versão X"), e metadados viram gambiarra em `appProperties`. Para sync bidirecional com celular, precisamos de três coisas: **armazenamento de blobs**, **um log de mudanças ordenado com controle otimista de versão** e **notificação push**.

| Opção | Prós | Contras |
|-------|------|---------|
| **Supabase** (Postgres + Storage S3 + Auth + Realtime) | Tudo num lugar; Postgres dá transações e o log de mudanças de graça; Storage fala protocolo S3; RLS isola usuários; SDK oficial para Kotlin/Swift/Flutter; **auto-hospedável** (sem lock-in). | Cliente Go oficial é imaturo (`supabase-go` v0.0.4) — usaremos PostgREST/RPC via HTTP + SDK S3 direto, sem SDK Supabase no Go. Plano free: 1 GB e 50 MB por arquivo. |
| Servidor Go próprio + S3 (R2/B2/MinIO) | Controle total, um binário a mais. | Precisa hospedar e operar servidor, auth e push por conta própria. |
| Firebase (Firestore + Storage) | Excelente no mobile, FCM nativo. | Não há SDK cliente Go (só Admin); cobrança por leitura em Firestore pune listagens de árvore; lock-in total. |
| P2P estilo Syncthing | Sem servidor. | Celular raramente está online junto com o PC; o app Android do Syncthing foi descontinuado em 2024. |

**Recomendação: Supabase**, mas atrás de uma interface `Remote` para não repetir o acoplamento atual ao Drive:

```go
type Remote interface {
    PutBlob(ctx context.Context, key string, r io.Reader, size int64) error
    GetBlob(ctx context.Context, key string) (io.ReadCloser, error)
    Commit(ctx context.Context, c Change) (Committed, error) // falha com ErrConflict se base_version mudou
    Changes(ctx context.Context, since int64, limit int) ([]Change, int64, error)
    Subscribe(ctx context.Context) (<-chan struct{}, error)   // "tem novidade"; fallback para polling
}
```

Implementações: `remote/supabase` (primeira), `remote/selfhosted` (futuro, servidor Go + S3) e, opcionalmente, `remote/gdrive` como destino só de backup.

### 3.1 Modelo de dados no servidor (Postgres)

```sql
create table devices (
  id uuid primary key default gen_random_uuid(),
  user_id uuid not null default auth.uid() references auth.users,
  name text not null, platform text not null,
  last_seen_at timestamptz
);

-- "Pastas" lógicas: no PC "ssh" -> ~/.ssh; no celular "ssh" -> pasta escolhida pelo usuário
create table roots (
  id uuid primary key default gen_random_uuid(),
  user_id uuid not null default auth.uid() references auth.users,
  name_enc bytea not null
);

create table files (
  id uuid primary key default gen_random_uuid(),
  user_id uuid not null default auth.uid() references auth.users,
  root_id uuid not null references roots on delete cascade,
  path_hmac bytea not null,   -- HMAC(chave, caminho relativo): busca sem revelar o nome
  path_enc  bytea not null,   -- caminho relativo cifrado
  version   bigint not null,
  blob_key  text,             -- null quando deleted
  size bigint, mode int, mtime timestamptz, content_hash bytea,
  deleted boolean not null default false,
  updated_by uuid references devices,
  unique (user_id, root_id, path_hmac)
);

create table changes (           -- log ordenado; cada device guarda o último seq aplicado
  seq bigserial primary key,
  user_id uuid not null, file_id uuid not null, version bigint not null,
  device_id uuid, created_at timestamptz default now()
);
```

- Escrita só via função RPC `commit_change(file_id, base_version, …)` em PL/pgSQL: verifica `version = base_version`, incrementa, insere em `changes` na mesma transação. Conflito → erro `40001`.
- RLS `user_id = auth.uid()` em todas as tabelas; bucket de Storage com política por prefixo `<user_id>/`.
- Realtime em `changes` (INSERT) acorda os clientes; Edge Function no mesmo evento dispara **FCM** para os celulares.
- Job agendado (`pg_cron`) apaga blobs sem referência e mantém as últimas N versões (histórico/restauração).

### 3.2 Criptografia ponta a ponta (obrigatória)

O caso de uso principal inclui `~/.ssh` e `~/.kube`. O servidor não pode ver conteúdo nem nomes:

- Chave mestra aleatória por conta, protegida por senha (Argon2id) + **chave de recuperação** impressa no setup; cópia cifrada da chave mestra fica no servidor para novos dispositivos.
- Conteúdo: XChaCha20-Poly1305 em blocos (streaming, arquivos grandes), chave por arquivo derivada da mestra.
- Caminhos: cifrados (`path_enc`) + `path_hmac` para lookup.
- Projetar isso **desde o início do schema** — migrar depois é muito mais caro.

## 4. Arquitetura do cliente

```
cmd/superpose/            CLI + daemon (urfave/cli v3)
internal/config/          YAML do usuário (somente leitura) + XDG dirs
internal/state/           SQLite local (modernc) + migrações: índice de arquivos, cursor, fila
internal/watch/           fsnotify recursivo + debounce + scanner periódico de reconciliação
internal/engine/          motor de sync: planeja upload/download/delete/conflito
internal/crypto/          E2E
internal/remote/          interface Remote + supabase/ (+ gdrive/ legado)
mobile/                   API exportada para gomobile (Android .aar / iOS .xcframework)
```

Princípios:

- **Sem estado global**; tudo recebe `context.Context` e dependências por construtor (testável).
- **Configuração ≠ estado**: o `watchers.yml` nunca é reescrito pelo programa; tokens vão para o keyring, IDs e cursores para o SQLite.
- **Índice local** por arquivo: `(root, path_rel, size, mtime, inode, content_hash, synced_version)`. É ele que permite:
  - detectar mudança real (hash) e ignorar eco do que acabamos de baixar;
  - reconciliar após o daemon ficar desligado ou após `IN_Q_OVERFLOW` (scan completo comparando com o índice);
  - sync inicial por *merge*, sem exigir pasta vazia (itens do roadmap "initial download" e "sync in place" viram um só).
- **Escrita atômica** no download: grava em `.superpose.tmp`, `fsync`, `rename`; aplica `mode` e `mtime`.
- **Conflitos**: se `Commit` retorna `ErrConflict`, o motor baixa a versão remota e salva a local como `nome (conflito de <device> 2026-10-02).ext`. Edição vence remoção.
- **Renomear** = delete + create no MVP (com dedup por `content_hash` o blob não é reenviado); detecção de rename por inode fica para depois.

## 5. Celular

**Android primeiro, iOS depois.**

- App **Kotlin + Jetpack Compose**, com o núcleo Go (`engine`, `crypto`, `remote`, `state`) compilado via `gomobile bind` para `.aar`. Mesma lógica de conflito e criptografia no PC e no celular — sem reimplementar o protocolo. (Alternativa: Flutter reimplementando o motor em Dart; mais rápido para UI, mas duplica a parte mais delicada.)
- Login com Supabase Auth (`supabase-kt` para a sessão; o núcleo Go recebe o JWT).
- Acesso a arquivos: pasta escolhida pelo usuário via Storage Access Framework (`ACTION_OPEN_DOCUMENT_TREE`). SAF é lento para scans grandes, então o índice local é ainda mais importante. `MANAGE_EXTERNAL_STORAGE` só para build fora da Play Store.
- Quando sincronizar:
  - push FCM (Edge Function) → `WorkManager` expedited job;
  - `WorkManager` periódico (mínimo 15 min) como rede de segurança;
  - com o app aberto, Realtime direto;
  - mudanças locais: `ContentObserver`/scan ao abrir e no job periódico (Android não tem inotify confiável em SAF).
- Dois modos por pasta: **espelho completo** (ex.: Documentos) e **sob demanda** (lista tudo, baixa ao tocar — útil para pastas grandes).
- **iOS**: `NSFileProviderReplicatedExtension` (aparece no app Arquivos), mesmo núcleo via `.xcframework`. É uma fase separada pela complexidade do File Provider.

## 6. Fases

### Fase 0 — Higiene (≈1 semana)
- [ ] Remover `build/go_build_zero_sync` e ignorar `build/`.
- [ ] `go 1.27`, atualizar dependências da seção 2, corrigir `go vet`, trocar `ioutil` e `log` por `os`/`slog`.
- [ ] GitHub Actions: `build`, `vet`, `golangci-lint`, `test -race`.
- [ ] Primeiros testes: `PathInIgnore`, expansão de `~`, parsing de máscara.
- [ ] Corrigir bugs #1, #2, #3, #5, #7, #9 (são pequenos e ajudam a entender o fluxo antes da refatoração).

### Fase 1 — Núcleo novo, ainda local (≈2–3 semanas)
- [ ] Estrutura de pacotes da seção 4, sem globais.
- [ ] `internal/watch` com fsnotify + debounce + scanner de reconciliação.
- [ ] `internal/state` com modernc/sqlite e migrações.
- [ ] `internal/engine` testado contra um `Remote` **em memória** (fake) — permite testar conflitos, renomes e quedas sem rede.
- [ ] Comandos: `superpose init`, `superpose run`, `superpose status`.

### Fase 2 — Supabase + sync bidirecional + E2E (≈3–4 semanas)
- [ ] Projeto Supabase, migrações SQL versionadas em `supabase/migrations`, RLS, bucket, função `commit_change`.
- [ ] `internal/crypto` e fluxo de chave mestra/recuperação.
- [ ] `remote/supabase`: PostgREST + S3 (multipart para >50 MB / uploads resumíveis), Realtime com fallback de polling.
- [ ] Login via device flow/magic link (`superpose login`), tokens no keyring, registro de device.
- [ ] Sync inicial por merge, download, aplicação de metadados, remoções, conflitos.
- [ ] Teste end-to-end com duas instâncias contra Supabase local (`supabase start`) na CI.
- [ ] Manter `remote/gdrive` só como backup opcional (ou descontinuar).

### Fase 3 — Android (≈4–6 semanas)
- [ ] Pacote `mobile/` com API mínima e estável para gomobile.
- [ ] App Kotlin: login, desbloqueio da chave, seleção de pastas (SAF), status, conflitos.
- [ ] WorkManager + FCM via Edge Function.
- [ ] Distribuição: APK nas releases do GitHub / F-Droid; Play Store depois.

### Fase 4 — Produto (contínuo)
- [ ] Serviço `systemd --user`, `launchd` (macOS) e Windows (fsnotify já cobre).
- [ ] Histórico/restauração de versões, lixeira, `superpose log`.
- [ ] Chunking por conteúdo (FastCDC) para arquivos grandes e delta upload.
- [ ] iOS (File Provider).
- [ ] `remote/selfhosted` (servidor Go + S3) para quem não quiser Supabase.

## 7. Riscos

- **Limites do inotify** (`fs.inotify.max_user_watches`) em árvores grandes → documentar e detectar o erro `ENOSPC`.
- **Plano free do Supabase**: 1 GB de Storage e 50 MB por arquivo; projetos inativos são pausados. Para uso pessoal contínuo, Pro ou auto-hospedagem.
- **Perda da chave E2E** = perda dos dados. A chave de recuperação precisa ser parte obrigatória do onboarding.
- **Android em segundo plano** (Doze, fabricantes agressivos) → FCM de alta prioridade + documentação de exceção de bateria.
- **gomobile** aceita só tipos simples na fronteira → manter a API `mobile/` pequena (strings, bytes, callbacks).

## 8. Decisões em aberto

1. Supabase hospedado ou auto-hospedado desde o início?
2. E2E obrigatório para todas as pastas (recomendado) ou opcional por pasta?
3. Android nativo (Kotlin + gomobile, recomendado) ou Flutter?
4. Manter o Google Drive como destino de backup ou removê-lo de vez?
