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

To get your API token, open [https://zerro.app/token](https://zerro.app/token), log in with your ZenMoney account, and copy the token shown on the page.

The config file `~/.config/zenmoney-mcp/config.json` is created on first run with defaults. No manual setup required beyond the token.

## Tools

### Sync

| Tool | What it does |
|------|--------------|
| `sync` | Incremental diff sync — fetches only entities changed since the last sync |
| `full_sync` | Full sync — resets cached sync state and re-downloads the full dataset; use to resolve inconsistencies or on first run |

### Accounts

| Tool | What it does |
|------|--------------|
| `list_accounts` | Fetch and list current financial accounts from ZenMoney; set `active_only=true` to exclude archived |
| `find_account` | Fetch current accounts from ZenMoney and return the first title match (case-insensitive) |

### Transactions

| Tool | What it does |
|------|--------------|
| `list_transactions` | Fetch current transactions from ZenMoney and return filtered results as `{items, total, offset, limit}`; `query` searches across both payee and comment |
| `create_transaction` | Fetch current account/tag data from ZenMoney, then create a new transaction; supports transfers via `to_account_id` |
| `update_transaction` | Fetch the current transaction from ZenMoney, apply only the provided changes, and update it by ID |
| `delete_transaction` | Fetch the current transaction from ZenMoney, delete it by ID, and return the deleted record for confirmation |

### Tags (Categories)

| Tool | What it does |
|------|--------------|
| `list_tags` | Fetch and list current category tags from ZenMoney |
| `find_tag` | Fetch current tags from ZenMoney and return the first title match (case-insensitive) |
| `create_tag` | Fetch current tags from ZenMoney, then create a category tag if needed; idempotent — returns an existing tag if the title already exists |

### Reference data

| Tool | What it does |
|------|--------------|
| `list_merchants` | Fetch and list current merchants (payees / counterparties) from ZenMoney |
| `list_budgets` | Fetch current monthly budgets from ZenMoney; optionally filter by month (`YYYY-MM`) |
| `list_reminders` | Fetch and list current recurring transaction reminders from ZenMoney |
| `list_instruments` | Fetch and list current currency instruments with exchange rates from ZenMoney |
| `get_instrument` | Fetch current instruments from ZenMoney and return the one matching a numeric ID |

### Categorisation

| Tool | What it does |
|------|--------------|
| `suggest_category` | Suggest a category tag for a transaction based on payee and/or comment, resolving returned tag IDs against current ZenMoney categories |

### Bulk operations

| Tool | What it does |
|------|--------------|
| `prepare_bulk_operations` | Fetch current transaction/account/tag state from ZenMoney, then validate and preview up to 20 create/update/delete operations without committing; returns a `preparation_id` |
| `execute_bulk_operations` | Commit a previously prepared bulk operation to ZenMoney; returns a summary |

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

- Read and lookup tools fetch current data from ZenMoney as needed; they do not rely only on the raw output of the last incremental `sync` call.
- Sync state is cached in `~/.config/zenmoney-mcp/sync_state.json` and is scoped to the configured API token. Delete it to force a clean full sync on next run.
- This project is not affiliated with ZenMoney or its parent company.

## Contributing

Bug fixes and clear improvements are welcome. Open an issue first for anything non-trivial.

## License

MIT
