# 🔔 Reminder

A web application for managing reminders with Telegram and Email delivery.

## Features

- 🔐 **Authentication** — Login/register with session management
- 🔑 **Password Recovery** — Reset via registered email
- 📋 **Full CRUD** — Create, list, edit, delete reminders
- 📅 **Calendar View** — Visual calendar with reminders on their due dates
- 📤 **Send** — Deliver reminders via Telegram, Email, or both
- 🏷️ **Tags & Priority** — Organize with tags (high/medium/low priority)
- 🔍 **Search & Filter** — Filter by priority, status (overdue/pending/sent)
- ✅ **Bulk Actions** — Select multiple reminders for batch send/delete
- 💾 **Persistent** — SQLite database, data survives restarts

## Requirements

- Go 1.22+
- GCC (for go-sqlite3 CGO compilation)

```bash
# Ubuntu/Debian
sudo apt install gcc

# macOS
xcode-select --install
```

## Install & Run

```bash
# 1. Get dependencies
go mod download

# 2. Build
go build -o reminder .

# 3. Run (defaults: port 8080, db file: reminder.db)
./reminder

# Custom options
./reminder --port 9000 --db /data/reminder.db --base-url http://myserver.com
```

Then open: **http://localhost:8080**

## Commands (CLI equivalents → Web UI)

| CLI Command | Web Equivalent |
|-------------|----------------|
| `reminder i` / `insert` | Click "New Reminder" button |
| `reminder l` / `list` | Main page shows all reminders |
| `reminder d` / `delete` | Click 🗑 on item or select + "Delete Selected" |
| `reminder c` / `clear` | Click "Clear All" button |
| `reminder s` / `send` | Click "Send" button, choose channel |
| `reminder cal` / `calendar` | Click "Calendar" in nav |

## Configuration

### Telegram Setup
1. Message `@BotFather` on Telegram → `/newbot`
2. Copy the bot token
3. Add bot to your group or start a private chat
4. Get your Chat ID via `@userinfobot`
5. Enter both in Settings → Telegram

### Email Setup (Gmail example)
1. Enable 2FA on your Google account
2. Generate an App Password: [Google App Passwords](https://support.google.com/accounts/answer/185833)
3. In Settings → Email:
   - Host: `smtp.gmail.com`
   - Port: `587`
   - User: `you@gmail.com`
   - Pass: your App Password

## Flags

```
--port       Port to listen on (default: 8080)
--db         Path to SQLite database file (default: reminder.db)
--secret     Session secret key (change this in production!)
--base-url   Base URL for password reset links (default: http://localhost:PORT)
```

## Production Deployment

```bash
# Set a strong secret key
./reminder --secret "$(openssl rand -hex 32)" --base-url https://reminder.yourdomain.com

# Or use environment-based config with a systemd service
```
