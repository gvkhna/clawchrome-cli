---
name: clawchrome-cli
description: Use when controlling or inspecting a browser with clawchrome-cli from a shell, including navigation, accessibility snapshots, clicking and filling elements by snapshot refs, screenshots, tabs, waits, form controls, keyboard input, and common browser workflows.
---

# clawchrome-cli

Use `clawchrome-cli` to drive a browser from the shell. The normal loop is: open or select a page, take a snapshot, act on `@eNN` refs from that snapshot, then snapshot again.

## Quick Start

```sh
clawchrome-cli open https://example.com
clawchrome-cli snapshot
clawchrome-cli click @e12
clawchrome-cli fill @e17 "search terms"
clawchrome-cli press Enter
clawchrome-cli screenshot ./page.png
```

Element commands use snapshot refs such as `@e12` or `@e27`. Run `clawchrome-cli snapshot` after navigation or page changes to get current refs.

## Help

- Run `clawchrome-cli --help` for the full command list.
- Run `clawchrome-cli <command> --help` or `clawchrome-cli <command> -h` before guessing flags.
- Subcommand help shows the required args, available flags, and examples for that specific command.

## Main Commands

- `open <url>`: navigate the current page and print a snapshot.
- `snapshot [--form] [--text] [--full]`: inspect the current page. Use `--form` for controls, `--text` for readable text, and `--full` when truncation hides needed refs.
- `click @eNN`: click a link, button, checkbox, or other interactive element.
- `fill @eNN <text>`: fill an input or textarea.
- `type <text>`: type into the currently focused element.
- `press <key>`: press keys such as `Enter`, `Tab`, `Escape`, or `ArrowDown`.
- `scroll <up|down|top|bottom>`: move around the page.
- `wait <ms|text>`: wait for a duration or for text to appear.
- `screenshot [path]`: save a screenshot. Without a path, the tool chooses a temp file and prints it.
- `pages`: list open tabs/pages.
- `newpage <url>`: open a new tab.
- `selectpage <id>`: switch tabs by ID from `pages`.
- `closepage <id>`: close a tab; the last page cannot be closed.
- `back`, `forward`, `reload`: browser history and reload actions.
- `hover @eNN` and `drag @eNN @eNN`: pointer interactions by snapshot ref.
- `fillform @eNN=<value>...`: fill several fields in one command.
- `form clear|check|uncheck|select|upload ...`: targeted input clearing, checkbox, select, and file upload helpers.
- `dialog accept|dismiss [text]`: handle alerts, confirms, and prompts.
- `resize <width> <height>`: set viewport size.
- `video start [path]` and `video stop`: record and stop page video.
- `start [url] [--token <token>] [--agent-name <name>]`: start the browsing session. `--token` saves HTTP auth for later commands; `--agent-name` saves agent metadata for runtime HTTP requests.
- `stop`: stop the bridge.
- `version`: print the installed version.
- `self-update`: update the CLI.

## Common Workflows

Navigate and act from the snapshot:

```sh
clawchrome-cli open https://example.com/login
clawchrome-cli snapshot --form
clawchrome-cli fill @e12 "user@example.com"
clawchrome-cli fill @e13 "correct horse battery staple"
clawchrome-cli click @e14
clawchrome-cli wait "Dashboard"
```

Work with tabs:

```sh
clawchrome-cli pages
clawchrome-cli newpage https://example.com/docs
clawchrome-cli selectpage 2
clawchrome-cli closepage 1
```

Capture page state:

```sh
clawchrome-cli snapshot --full
clawchrome-cli screenshot
clawchrome-cli screenshot ./checkout.png --full-page
clawchrome-cli screenshot ./button.png --uid @e29
```

Configure HTTP auth:

```sh
clawchrome-cli start --token "$CLAWCHROME_TOKEN" --agent-name codex-worker
clawchrome-cli status
```

Handle forms and dialogs:

```sh
clawchrome-cli form clear @e17
clawchrome-cli form check @e18
clawchrome-cli form select @e19 "United States"
clawchrome-cli form upload @e20 ./avatar.png
clawchrome-cli dialog accept
```
