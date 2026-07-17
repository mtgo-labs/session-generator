# tgconv — Telegram Session Generator

CLI tool for converting, inspecting, and generating Telegram session strings.
Reads SQLite session files, authenticates with Telegram, and exports to any
supported library format.

Part of the [mtgo-labs](https://github.com/mtgo-labs) ecosystem. Built on
[session-converter](https://github.com/mtgo-labs/session-converter) (format
conversion), [mtgo](https://github.com/mtgo-labs/mtgo) (MTProto client), and
[device-manager](https://github.com/mtgo-labs/device-manager) (device profiles for relogin).

## Install

```bash
go install github.com/mtgo-labs/session-generator@latest
```

Or build from source:

```bash
git clone https://github.com/mtgo-labs/session-generator
cd session-generator
go build -o tgconv .
```

## Commands

```
tgconv convert <session-string> -t <format> [-f <format>] [flags]
tgconv info <session-string> [-f <format>]
tgconv from-file <sqlite-path> [-t <format>] [flags]
tgconv generate --api-id <id> --api-hash <hash> [--bot-token <token> | --phone <number>] [-t <format>]
tgconv relogin <session-string> --api-id <id> --api-hash <hash> [-t <format>] [--device <type>] [--device-id <id>]
tgconv list
```

| Command    | Description                                              |
|------------|----------------------------------------------------------|
| `convert`  | Convert a session string to another format (auto-detects source) |
| `info`     | Show decoded session information (DC, user ID, auth key) |
| `from-file`| Read a SQLite session file (Telethon/Pyrogram) and export as a string |
| `generate` | Generate a new session by authenticating with Telegram  |
| `relogin`  | Re-authenticate an existing session as a fresh login with a new device |
| `list`     | List supported formats                                   |

## Supported Formats

telethon, pyrogram, gramjs, mtcute, mtkruto, gogram, gotgproto

## Examples

### Convert a session string

```bash
# Auto-detect source, convert to Pyrogram
tgconv convert "1BVtsOH8Bu..." -t pyrogram

# Specify source format explicitly
tgconv convert "1BVtsOH8Bu..." -f telethon -t gramjs

# Add API ID / user ID for Pyrogram or mtcute output
tgconv convert "1BVtsOH8Bu..." -t pyrogram --api-id 12345 --user-id 67890
```

### Inspect a session

```bash
tgconv info "1BVtsOH8Bu..."
# DC: 2
# User ID: 67890
# Auth Key: a4f1...
# Is Bot: false
```

### Read a SQLite session file

```bash
# Export Telethon .session file as a string
tgconv from-file ~/sessions/account.session

# Convert to another format
tgconv from-file ~/sessions/account.session -t pyrogram
```

Auto-detects whether the SQLite file is from Telethon (tables: `sessions`,
`entities`, `version`) or Pyrogram (tables: `sessions`, `peers`, `version`).

### Generate a new session

```bash
# Bot session
tgconv generate --api-id 12345 --api-hash abc123 --bot-token 123:ABC -t telethon

# User session (interactive code / 2FA prompts)
tgconv generate --api-id 12345 --api-hash abc123 --phone +1234567890 -t pyrogram
```

The `generate` command authenticates via MTProto using
[mtgo](https://github.com/mtgo-labs/mtgo), exports the session in Pyrogram
format internally, then converts to the requested output format.

### Re-authenticate a session (relogin)

```bash
# Re-authenticate a Telethon session as a fresh Android login
tgconv relogin "1BVtsOH8Bu..." --api-id 12345 --api-hash abc123 -t pyrogram

# Use a different device profile
tgconv relogin "1BVtsOH8Bu..." --api-id 12345 --api-hash abc123 --device ios

# Deterministic device identity (same ID = same model/version)
tgconv relogin "1BVtsOH8Bu..." --api-id 12345 --api-hash abc123 --device desktop --device-id my-session-1

# From a SQLite session file
tgconv relogin ~/sessions/account.session --api-id 12345 --api-hash abc123
```

The `relogin` command:
1. Connects with the existing session to retrieve the account's phone number.
2. Generates a fresh device profile via [device-manager](https://github.com/mtgo-labs/device-manager).
3. Starts a completely new authentication flow (new auth key server-side).
4. Prompts for the login code and 2FA password (if enabled).
5. Exports the brand-new session in the requested format.

Device types: `android`, `android_x`, `ios`, `macos`, `windows`, `linux`,
`desktop`, `web_z`, `web_k`, `webogram`.

### Go API

The `relogin` package is importable for programmatic use:

```go
import (
    "github.com/mtgo-labs/session-generator/relogin"
    device "github.com/mtgo-labs/device-manager"
)

result, err := relogin.Relogin(ctx, relogin.Options{
    APIID:      12345,
    APIHash:    "abc123",
    Session:    "1BVtsOH8Bu...",
    Format:     "pyrogram",
    DeviceType: device.Android,
})
```

## License

MIT
