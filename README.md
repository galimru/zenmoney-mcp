<div align="center">

**MCP server for the ZenMoney personal finance platform**

*Ask your AI assistant to browse accounts, manage transactions, and import data — right from the chat*

<p>
  <img src="https://img.shields.io/github/v/release/galimru/zenmoney-mcp" alt="Latest release">
  <img src="https://img.shields.io/badge/MCP-compatible-blueviolet" alt="MCP">
  <img src="https://img.shields.io/badge/Go-1.26+-00ADD8?style=flat&logo=go&logoColor=white" alt="Go">
  <img src="https://img.shields.io/badge/license-MIT-blue" alt="License">
</p>

</div>

---

Connect your AI assistant to ZenMoney. List and search transactions, create and update records, manage category tags, browse budgets and merchants, and run bulk imports — all through natural conversation.

## Quick Start

**1. Install**

Download a binary from the [releases page](https://github.com/galimru/zenmoney-mcp/releases).

Or build from source:

```bash
git clone https://github.com/galimru/zenmoney-mcp.git
cd zenmoney-mcp
make install
```

**2. Connect to Claude Desktop**

Add to `claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "zenmoney": {
      "command": "/usr/local/bin/zenmoney-mcp",
      "env": {
        "ZENMONEY_TOKEN": "your-api-token"
      }
    }
  }
}
```

The config file `~/.config/zenmoney-mcp/config.json` is created on first run with defaults. No manual setup required beyond the token.

## Tools

### Sync

| Tool | What it does |
|------|--------------|
| `sync` | Incremental sync — fetches only changes since the last sync |
| `full_sync` | Full sync — resets cached state and re-downloads all data; use to resolve inconsistencies or on first run |

### Accounts

| Tool | What it does |
|------|--------------|
| `list_accounts` | List financial accounts; set `active_only=true` to exclude archived |
| `find_account` | Find an account by title (case-insensitive, returns first match) |

### Transactions

| Tool | What it does |
|------|--------------|
| `list_transactions` | List transactions with optional filters; returns `{items, total, offset, limit}` |
| `create_transaction` | Create a new transaction; supports transfers via `to_account_id` |
| `update_transaction` | Update an existing transaction by ID; only provided fields are changed |
| `delete_transaction` | Delete a transaction by ID; returns the deleted record for confirmation |

### Tags (Categories)

| Tool | What it does |
|------|--------------|
| `list_tags` | List all category tags |
| `find_tag` | Find a tag by title (case-insensitive, returns first match) |
| `create_tag` | Create a category tag; idempotent — returns existing tag if title already exists |

### Reference data

| Tool | What it does |
|------|--------------|
| `list_merchants` | List all merchants (payees / counterparties) |
| `list_budgets` | List monthly budgets; optionally filter by month (`YYYY-MM`) |
| `list_reminders` | List all recurring transaction reminders |
| `list_instruments` | List currency instruments with current exchange rates |
| `get_instrument` | Get a specific currency instrument by numeric ID |

### Categorisation

| Tool | What it does |
|------|--------------|
| `suggest_category` | Suggest a category tag for a transaction based on payee and/or comment |

### Bulk operations

| Tool | What it does |
|------|--------------|
| `prepare_bulk_operations` | Validate and preview up to 20 create/update/delete operations without committing; returns a `preparation_id` |
| `execute_bulk_operations` | Commit a previously prepared bulk operation; returns a summary |

### Example workflow — import a bank statement

1. Run `full_sync` to make sure local data is fresh.
2. Use `list_accounts` to find the account ID for your bank account.
3. Use `find_tag` (or `create_tag`) to resolve category names to IDs.
4. Use `prepare_bulk_operations` to preview the transactions parsed from the statement.
5. Review the preview, then call `execute_bulk_operations` with the returned `preparation_id`.

## Configuration

**Environment variables**

| Variable | Required | Description |
|----------|----------|-------------|
| `ZENMONEY_TOKEN` | Yes | Your ZenMoney API token |

**Config file**

`~/.config/zenmoney-mcp/config.json` is created on first run with defaults:

```json
{
  "transaction_limit": 100,
  "max_bulk_operations": 20
}
```

| Field | Required | Description |
|-------|----------|-------------|
| `transaction_limit` | No | Default page size for `list_transactions`. `0` is treated as the default value `100`. |
| `max_bulk_operations` | No | Maximum number of operations per `prepare_bulk_operations` call. Must be ≤ 100. Default: `20`. |

## Notes

- Sync state is cached in `~/.config/zenmoney-mcp/sync_state.json`. Delete it to force a clean full sync on next run.
- This project is not affiliated with ZenMoney or its parent company.

## Contributing

Bug fixes and clear improvements are welcome. Open an issue first for anything non-trivial.

## License

MIT
