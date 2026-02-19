# 🔔 Reminder

Aplicação web para gerenciar lembretes com notificações nativas do navegador (Web Push), recorrência automática, calendário visual e autenticação de usuários.

## Funcionalidades

- 🔐 **Autenticação** — Login, cadastro e recuperação de senha por link
- 📋 **CRUD completo** — Criar, listar, editar e excluir lembretes
- 🔁 **Recorrência** — Diário, Seg–Sex, Semanal, Quinzenal, Mensal, Anual
- 🔔 **Notificações Web Push** — Alertas nativos do navegador no horário do lembrete
- 📅 **Calendário** — Visualização mensal com lembretes nos dias
- 🏷️ **Tags & Prioridade** — Alta / Média / Baixa com filtragem
- 🔍 **Busca & Filtros** — Por texto, prioridade, status e recorrência
- ✅ **Ações em lote** — Selecionar vários lembretes para excluir
- 💾 **Persistência** — SQLite com WAL mode, sobrevive a reinicializações
- 📝 **Logs estruturados** — Níveis DEBUG/INFO/WARN/ERROR com cores e caller info

## Requisitos

- **Go 1.22+**
- **GCC** (necessário para compilar o driver SQLite via CGO)

```bash
# Ubuntu / Debian
sudo apt install gcc

# macOS
xcode-select --install

# Windows
# Instale o TDM-GCC: https://jmeubank.github.io/tdm-gcc/
```

## Instalação e execução

```bash
# 1. Baixar dependências
go mod download

# 2. Compilar
go build -o reminder .

# 3. Executar (padrão: porta 8080, banco reminder.db)
./reminder
```

Acesse: **http://localhost:8080**

## Opções de linha de comando

```
--port       Porta do servidor HTTP          (padrão: 8080)
--db         Caminho do banco SQLite         (padrão: reminder.db)
--secret     Chave secreta de sessão         (padrão: string insegura — troque em produção!)
--base-url   URL base para links de e-mail   (padrão: http://localhost:PORT)
--debug      Ativar logs nível DEBUG         (padrão: false)
--log-file   Gravar logs em arquivo          (padrão: stderr)
```

### Exemplos

```bash
# Desenvolvimento com logs detalhados
./reminder --debug

# Produção com chave aleatória e logs em arquivo
./reminder \
  --port 8080 \
  --secret "$(openssl rand -hex 32)" \
  --base-url https://reminder.meudominio.com \
  --log-file /var/log/reminder.log

# Banco em diretório separado
./reminder --db /data/reminder.db
```

## Logs

O servidor emite logs estruturados com timestamp, nível, arquivo:linha e mensagem:

```
2026-02-19 14:03:01.042 INFO  main/main.go:88    🔔  Reminder server listening on :8080
2026-02-19 14:03:01.042 INFO  main/main.go:89        URL      : http://localhost:8080
2026-02-19 14:03:01.042 INFO  main/main.go:90        Database : reminder.db
2026-02-19 14:03:05.210 INFO  logger/logger.go:98   GET    /                        → 200  2ms
2026-02-19 14:03:06.315 INFO  handlers/handlers.go:75  Login OK: username="alice" userID=1
2026-02-19 14:03:10.001 WARN  handlers/handlers.go:72  Login FAILED: username="x" ip=127.0.0.1:54321
```

Use `--debug` para ver queries SQL, renders de templates e verificações de autenticação.

## Notificações Web Push

As notificações funcionam diretamente pelo navegador, sem serviços externos:

1. Ao abrir o app, um banner pede permissão — clique **Ativar**
2. O app verifica os lembretes a cada **30 segundos**
3. Quando um lembrete vence (ou está a menos de 5 minutos do vencimento), o navegador exibe uma notificação nativa
4. Lembretes **recorrentes** têm seu `due_date` avançado automaticamente após cada notificação

Para desativar: `Configurações do navegador → Privacidade → Notificações → [url do app] → Bloquear`

> **Nota:** notificações funcionam apenas enquanto a aba estiver aberta. Para notificações em background, considere um Service Worker.

## Recorrência

| Opção      | Comportamento                                          |
|------------|--------------------------------------------------------|
| Uma vez    | Dispara uma única notificação                          |
| Diário     | Avança 1 dia a cada notificação                        |
| Seg–Sex    | Avança para o próximo dia útil (pula sábado/domingo)   |
| Semanal    | Avança 7 dias                                          |
| Quinzenal  | Avança 14 dias                                         |
| Mensal     | Avança 1 mês (mesmo dia do mês)                        |
| Anual      | Avança 1 ano (mesma data)                              |

## Recuperação de senha

Como o app não tem servidor SMTP configurado por padrão, o link de redefinição é exibido diretamente na tela após solicitar a recuperação. Em produção, integre um serviço SMTP editando `internal/handlers/handlers.go` → `ForgotPasswordHandler`.

## Deploy com systemd

```ini
# /etc/systemd/system/reminder.service
[Unit]
Description=Reminder Web App
After=network.target

[Service]
Type=simple
User=www-data
WorkingDirectory=/opt/reminder
ExecStart=/opt/reminder/reminder \
    --port 8080 \
    --db /opt/reminder/data/reminder.db \
    --secret TROQUE_POR_CHAVE_FORTE \
    --base-url https://reminder.meudominio.com \
    --log-file /var/log/reminder.log
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl enable --now reminder
sudo journalctl -u reminder -f
```

## Estrutura do projeto

```
reminder/
├── main.go                        # Servidor HTTP, router, templates, flags
├── go.mod
├── templates/
│   ├── base.html                  # CSS e JS compartilhados (base_css, base_js)
│   ├── login.html
│   ├── register.html
│   ├── forgot.html
│   ├── reset.html
│   ├── index.html                 # Lista principal + modal + web push
│   ├── calendar.html
│   └── settings.html
├── static/
│   └── css/app.css
└── internal/
    ├── auth/auth.go               # Sessões, bcrypt, tokens de reset
    ├── db/db.go                   # SQLite, migrations, CRUD, recorrência
    ├── handlers/handlers.go       # Handlers HTTP e API REST
    └── logger/logger.go           # Logger estruturado com cores e middleware HTTP
```

## API REST

| Método   | Rota                              | Descrição                          |
|----------|-----------------------------------|------------------------------------|
| `GET`    | `/api/reminders`                  | Listar todos os lembretes          |
| `POST`   | `/api/reminders`                  | Criar lembrete                     |
| `PUT`    | `/api/reminders/{id}`             | Atualizar lembrete                 |
| `DELETE` | `/api/reminders/{id}`             | Excluir lembrete                   |
| `POST`   | `/api/reminders/{id}/notify`      | Marcar como notificado (avança data recorrente) |
| `POST`   | `/api/reminders/bulk-delete`      | Excluir vários (`{"ids":[1,2,3]}`) |
| `DELETE` | `/api/reminders/clear`            | Excluir todos                      |