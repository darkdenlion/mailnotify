# mailnotify

A TUI (Terminal User Interface) for viewing unread emails from Apple Mail on macOS.

![Built with Go](https://img.shields.io/badge/Built%20with-Go-00ADD8?style=flat&logo=go)

## Features

- View unread emails from Apple Mail inbox
- Read full email content directly in the terminal
- Auto-refresh every 10 seconds
- Searchable/filterable email list
- Keyboard-driven navigation

## Requirements

- macOS
- Apple Mail app
- Go 1.21+ (for building)

## Installation

```bash
git clone https://github.com/darkdenlion/mailnotify.git
cd mailnotify
go build -o mailnotify
```

## Usage

```bash
./mailnotify
```

On first run, macOS will prompt for automation permissions. Grant access in:
**System Settings → Privacy & Security → Automation → Terminal → Mail**

## Controls

### List View
| Key | Action |
|-----|--------|
| `↑/↓` | Navigate emails |
| `Enter` | Open email to read content |
| `/` | Search/filter emails |
| `r` | Manual refresh |
| `q` | Quit |

### Detail View
| Key | Action |
|-----|--------|
| `↑/↓` | Scroll email content |
| `q` / `Esc` | Back to list |

## How It Works

Uses AppleScript via `osascript` to communicate with Apple Mail and fetch:
- Unread message list (sender, subject, date)
- Full email content (plain text)

## License

MIT
