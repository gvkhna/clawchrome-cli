# clawchrome-cli

`clawchrome-cli` is a Go CLI that mirrors the high-level shape of `chrome-devtools-axi`:

- TOON-formatted output
- a persistent local bridge process
- `chrome-devtools-mcp` launched over stdio
- a browser-oriented CLI with AXI-style ergonomics

This subtree is maintained inside the parent monorepo, but the Go project itself is self-contained:

- its own `go.mod`
- its own `mise.toml`
- no `Containerfile`

## Development

```sh
mise run fmt
mise run mod:tidy
mise run test
mise run build
```

The main binary is built to `./.bin/clawchrome-cli`.

## Install

Install the latest release:

```sh
curl -fsSL https://raw.githubusercontent.com/gvkhna/clawchrome-cli/main/install.sh | sh
```

Install a specific version:

```sh
curl -fsSL https://raw.githubusercontent.com/gvkhna/clawchrome-cli/main/install.sh | sh -s -- --version v0.1.0
```

By default the installer writes to `~/.local/bin`.

## Update

Print the current version:

```sh
clawchrome-cli version
```

Update to the latest release:

```sh
clawchrome-cli self-update
```

Update to a specific release:

```sh
clawchrome-cli self-update v0.1.0
```

## Releases

This repo is configured for GoReleaser-based releases.

Local validation:

```sh
mise run release:check
mise run release:snapshot
```

GitHub releases:

- pushing a `v*` tag runs `.github/workflows/release.yml`
- GoReleaser publishes raw release binaries for Linux, Windows, and macOS
- macOS artifacts are published separately for `darwin/amd64` and `darwin/arm64`

## Notes

This is the Go implementation scaffold and initial translation target for `chrome-devtools-axi`.
The command surface and bridge architecture are aligned with the JavaScript implementation, while staying idiomatic to Go.

## Supported Normal-Mode APIs

These are the normal-mode CDP MCP APIs we intend to support.

- `[-] click` - Clicks on the provided element
- `[-] close_page` - Closes the page by its index. The last open page cannot be closed.
- `[-] drag` - Drag an element onto another element
- `[-] fill` - Type text into an input, text area or select an option from a select element.
- `[-] fill_form` - Fill out multiple form elements at once
- `[-] handle_dialog` - Handle an open browser dialog
- `[-] hover` - Hover over the provided element
- `[-] list_pages` - Get a list of pages open in the browser
- `[-] navigate_page` - Go to a URL, back, forward, or reload
- `[-] new_page` - Open a new tab and load a URL
- `[-] press_key` - Press a key or key combination
- `[-] resize_page` - Resize the selected page window
- `[-] select_page` - Select a page for future tool calls
- `[-] take_screenshot` - Take a page or element screenshot
- `[-] take_snapshot` - Take a text accessibility snapshot
- `[-] type_text` - Type text into a previously focused input
- `[-] upload_file` - Upload a file through a provided element
- `[-] wait_for` - Wait for text to appear on the selected page

## Supported Condition-Gated Normal-Mode APIs

These tools are not part of the default public surface, but they are still in scope for support.

- `[-] click_at` - Coordinate click helper gated by `--experimental-vision`
- `[-] get_tab_id` - Read the Chrome tab ID for a page
- `[-] screencast_start` - Start mp4 screencast recording
- `[-] screencast_stop` - Stop the active screencast recording

## Supported Slim-Mode APIs

Slim mode replaces the normal tool surface with these alternate APIs. They are in scope when the slim surface is exposed.

- `[-] navigate` - Load a URL
- `[-] evaluate` - Evaluate a JavaScript script
- `[-] screenshot` - Take a screenshot

## Explicitly Out Of Scope Upstream CDP MCP APIs

These upstream CDP MCP APIs are audited for compatibility reference only. They are not runtime support commitments.

- `emulate`
- `evaluate_script`
- `get_console_message`
- `get_network_request`
- `lighthouse_audit`
- `list_console_messages`
- `list_network_requests`
- `performance_analyze_insight`
- `performance_start_trace`
- `performance_stop_trace`
- `take_memory_snapshot`
- `install_extension`
- `uninstall_extension`
- `list_extensions`
- `reload_extension`
- `trigger_extension_action`
- `list_in_page_tools`
- `execute_in_page_tool`
