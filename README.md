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

Connect your AI assistant to ZenMoney. Search transactions, add and edit records, auto-categorize uncategorized activity, and import structured batches safely from the chat.

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

### Accounts

| Tool | What it does |
|------|--------------|
| `list_accounts` | Fetch and list current financial accounts from ZenMoney; archived accounts are hidden unless `show_archived=true` |
| `find_accounts` | Find accounts by title, with exact matches returned first and substring matches after |

### Transactions

| Tool | What it does |
|------|--------------|
| `find_transactions` | Search transactions by date, account, category, amount, payee, query, or type and return paginated results |
| `list_uncategorized_transactions` | List transactions without categories |
| `add_transaction` | Create a transaction from type, date, amount, `account_id`, and optional category, payee, comment, currency, or `to_account_id` fields |
| `edit_transaction` | Update a transaction by ID with optional field changes, using explicit account IDs for account changes and clear flags for payee, comment, or category |
| `remove_transaction` | Delete a transaction by ID |

### Categories

| Tool | What it does |
|------|--------------|
| `find_categories` | Find categories by title, or return categories up to the limit when no query is provided |
| `add_category` | Create a category if needed and return the existing one when the same title already exists |

### Categorisation

| Tool | What it does |
|------|--------------|
| `suggest_transaction_categories` | Suggest categories for the given transaction IDs |

### Import workflow

| Tool | What it does |
|------|--------------|
| `import_transactions` | Import a structured batch of rows in one call; if any row is invalid, duplicate, or needs review, nothing is imported and the response returns invalid rows |

### Example workflow — import structured rows

1. Use `find_accounts` to resolve the account ID you want to import into.
2. Prepare structured rows with `date`, `amount`, `type`, and optional `payee`, `comment`, `category`, `to_account_id`, and `external_id`.
3. Call `import_transactions` with `account_id` and `rows`.
4. If the response says nothing was imported, review the returned row indexes and reasons, fix the rows, and retry the full batch.
5. If the response says import succeeded, it returns only a compact summary with the imported count.

## Configuration

**Environment variables**

| Variable | Required | Description |
|----------|----------|-------------|
| `ZENMONEY_TOKEN` | Yes | Your ZenMoney API token |

**Config file**

`~/.config/zenmoney-mcp/config.json` is created on first run with defaults:

```json
{
  "transaction_limit": 100
}
```

| Field | Required | Description |
|-------|----------|-------------|
| `transaction_limit` | No | Default page size for `find_transactions`. `0` is treated as the default value `100`. |

## Notes

- Read and lookup tools fetch current data from ZenMoney as needed; they do not rely only on the raw output of the last incremental `sync` call.
- Sync state is cached in `~/.config/zenmoney-mcp/sync_state.json` and is scoped to the configured API token. Delete it to force a clean full sync on next run.
- This project is not affiliated with ZenMoney or its parent company.

## Contributing

Bug fixes and clear improvements are welcome. Open an issue first for anything non-trivial.

## License

MIT
