# Data Explorer — Storage.sol-Aware Blockchain Indexer for Akave

A domain-specific indexer and API built on top of the Akave public RPC that translates raw `Storage.sol` contract activity into human-readable, queryable storage actions — uploads, updates, deletions, registrations — instead of generic transactions.

> **Part of the [Akave PLDG](https://github.com/DarkLord017/akave-pldg)**

---

## The Problem

Akave's existing explorer ([explorer.akave.ai](https://explorer.akave.ai), powered by Blockscout) is great for generic blockchain exploration, but it exposes storage contract activity only as:

- Raw transactions and method selectors
- Encoded input data
- Uninterpreted log events

---

## The Solution

**Data Explorer** is an indexer + API service that:

1. Connects to the Akave public RPC
2. Pulls blocks and filters transactions to `Storage.sol` contract activity
3. Decodes method calls and emitted events using the contract ABI
4. Stores normalized, domain-meaningful records in a database
5. Exposes a queryable REST API for storage-centric views

Instead of:
> `"Transaction 0xabc… called contract 0xdef…"`

You get:
> `"Storage.uploadFile() by 0x…, CID=…, size=…, bucket=…, status=…"`

---

## 🏗️ Architecture

```
Akave Public RPC
      │
      ▼
┌─────────────┐      ┌──────────────────┐      ┌─────────────┐
│   Indexer   │─────▶│   Database (DB)  │◀─────│  API Layer  │
│   (Go)      │      │ Postgres/SQLite  │      │  REST/JSON  │
└─────────────┘      └──────────────────┘      └─────────────┘
```

### Indexer (Go)
- Connects to the Akave public RPC endpoint
- Tracks latest indexed block with reorg-safe windowing
- Filters transactions to `Storage.sol` contract address
- Decodes function selectors → method names (via ABI)
- Decodes input parameters and emitted events into typed structures

### Database
- PostgreSQL (preferred) or SQLite for MVP
- Core tables: `contracts`, `methods`, `events`, `actions`, `indexing_state`

### API Layer
- REST endpoints for querying decoded storage activity

```
GET /actions?method=upload&fromBlock=...
GET /actions/blocknumber/txnumber
GET /methods
GET /contracts
```

---

## 🚀 Getting Started

### Prerequisites

- Go 1.21+
- PostgreSQL (or SQLite for local dev)
- Access to Akave public RPC

### Network Info

| Resource | Value |
|---|---|
| Public RPC | `https://c6-us.akave.ai/ext/bc/56g16Hr1SHQRzdM8JLm3GKYv7APVHY8T2TyeZLvDVzCaTRS7W/rpc` |
| Explorer (Blockscout) | https://explorer.akave.ai |
| Faucet | https://faucet.akave.ai |

### Setup

```bash
# Clone the repo
git clone https://github.com/DarkLord017/akave-pldg.git
cd akave-pldg/data-explorer

# docker
make docker-build
make docker-run
```

## 🔗 Resources

- [Storage.sol ABI](https://github.com/akave-ai/akavesdk/blob/main/private/ipc/contracts/storage.go#L75)
- [Akave Docs](https://docs.akave.ai)
- [Decoding non-indexed events (Go)](https://ethereum.stackexchange.com/questions/28637/how-to-decode-log-data-in-go)
- [RLP decoding transactions (go-ethereum)](https://github.com/ethereum/go-ethereum/blob/master/core/types/transaction.go)
- [Tracking Issue #1](https://github.com/DarkLord017/akave-pldg/issues/1)
