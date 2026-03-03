package database

import (
	"context"
	"data-explorer/utils"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"time"

	_ "github.com/lib/pq"
)

type Config struct {
	Host     string
	Port     int
	User     string
	Password string
	DBName   string
	SSLMode  string
	MaxConns int
	MinConns int
}

func DefaultConfig() *Config {
	return &Config{
		Host:     "localhost",
		Port:     5432,
		User:     "indexer",
		Password: "indexer_password",
		DBName:   "blockchain_explorer",
		SSLMode:  "disable",
		MaxConns: 25,
		MinConns: 5,
	}
}

type DB struct {
	conn *sql.DB
}

func NewDB(cfg *Config) (*DB, error) {
	connStr := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.DBName, cfg.SSLMode,
	)

	conn, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	conn.SetMaxOpenConns(cfg.MaxConns)
	conn.SetMaxIdleConns(cfg.MinConns)
	conn.SetConnMaxLifetime(time.Hour)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := conn.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	log.Printf("Successfully connected to PostgreSQL database: %s", cfg.DBName)
	return &DB{conn: conn}, nil
}

func (db *DB) Close() error {
	if db.conn != nil {
		return db.conn.Close()
	}
	return nil
}

func (db *DB) BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {
	return db.conn.BeginTx(ctx, opts)
}

func (db *DB) InsertBlock(ctx context.Context, block *utils.Block) error {
	query := `
		INSERT INTO blocks (num, hash, parent_hash, timestamp)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (num) DO NOTHING
	`
	_, err := db.conn.ExecContext(ctx, query,
		block.Num,
		block.Hash,
		block.ParentHash,
		block.Timestamp,
	)
	if err != nil {
		return fmt.Errorf("failed to insert block %d: %w", block.Num, err)
	}
	return nil
}

func (db *DB) InsertTransaction(ctx context.Context, blockNum int64, blockHash []byte, tx *utils.DecodedTx) error {
	txParamsJSON, err := json.Marshal(tx.Params)
	if err != nil {
		return fmt.Errorf("failed to marshal tx params: %w", err)
	}

	var valueStr *string
	if tx.Value != nil {
		v := tx.Value.String()
		valueStr = &v
	}

	query := `
		SELECT pg_advisory_xact_lock(hashtext($1::text));
		INSERT INTO actions (
			block_num, block_hash, tx_hash, tx_index,
			from_addr, to_addr, contract,
			method, tx_params, value, events
		)
		VALUES ($2, $3, $4, $5, $6, $7, $8, $9, $10, $11, COALESCE((SELECT events FROM actions WHERE block_num = $2 AND tx_hash = $4), '[]'::jsonb))
		ON CONFLICT (block_num, tx_hash) DO UPDATE SET
			method = EXCLUDED.method,
			tx_params = EXCLUDED.tx_params,
			from_addr = EXCLUDED.from_addr,
			to_addr = EXCLUDED.to_addr,
			contract = EXCLUDED.contract,
			value = EXCLUDED.value
	`

	_, err = db.conn.ExecContext(ctx, query,
		tx.TxHash.Hex(),
		blockNum,
		blockHash,
		tx.TxHash.Bytes(),
		tx.From.Bytes(),
		tx.To.Bytes(),
		tx.To.Bytes(),
		tx.MethodName,
		txParamsJSON,
		valueStr,
	)
	if err != nil {
		return fmt.Errorf("failed to insert transaction: %w", err)
	}
	return nil
}

func (db *DB) UpdateEventsForTransaction(ctx context.Context, blockNum int64, txHash []byte, events []*utils.DecodedEvent) error {
	// Sanitize events before marshaling
	sanitizedEvents := make([]*utils.DecodedEvent, len(events))
	for i, ev := range events {
		sanitizedEvents[i] = utils.SanitizeEvent(ev)
	}

	eventsJSON, err := json.Marshal(sanitizedEvents)
	if err != nil {
		return fmt.Errorf("failed to marshal events: %w", err)
	}

	// Sanitize JSON string
	jsonStr := utils.SanitizeJSONString(string(eventsJSON))

	query := `
		SELECT pg_advisory_xact_lock(hashtext($1::text));
		INSERT INTO actions (block_num, block_hash, tx_hash, tx_index, from_addr, to_addr, contract, events)
		VALUES ($2, $3, $4, 0, $4, $4, $4, $5::jsonb)
		ON CONFLICT (block_num, tx_hash) DO UPDATE SET
			events = COALESCE(actions.events, '[]'::jsonb) || EXCLUDED.events
	`

	_, err = db.conn.ExecContext(ctx, query, string(txHash), blockNum, []byte{}, txHash, jsonStr)
	if err != nil {
		return fmt.Errorf("failed to update events: %w", err)
	}
	return nil
}

func (db *DB) InsertTransactionBatch(ctx context.Context, blockNum int64, blockHash []byte, txs []*utils.DecodedTx) error {
	if len(txs) == 0 {
		return nil
	}

	tx, err := db.conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	defer func() {
		if err != nil {
			if rbErr := tx.Rollback(); rbErr != nil {
				err = fmt.Errorf("tx err: %v, rollback err: %w", err, rbErr)
			}
		}
	}()

	for _, decodedTx := range txs {
		txParamsJSON, err := json.Marshal(decodedTx.Params)
		if err != nil {
			return fmt.Errorf("failed to marshal tx params: %w", err)
		}

		var valueStr *string
		if decodedTx.Value != nil {
			v := decodedTx.Value.String()
			valueStr = &v
		}

		query := `
			SELECT pg_advisory_xact_lock(hashtext($1::text));
			INSERT INTO actions (
				block_num, block_hash, tx_hash, tx_index,
				from_addr, to_addr, contract,
				method, tx_params, value, events
			)
			VALUES ($2, $3, $4, $5, $6, $7, $8, $9, $10, $11, COALESCE((SELECT events FROM actions WHERE block_num = $2 AND tx_hash = $4), '[]'::jsonb))
			ON CONFLICT (block_num, tx_hash) DO UPDATE SET
				method = EXCLUDED.method,
				tx_params = EXCLUDED.tx_params,
				from_addr = EXCLUDED.from_addr,
				to_addr = EXCLUDED.to_addr,
				contract = EXCLUDED.contract,
				value = EXCLUDED.value
		`

		_, err = tx.ExecContext(ctx, query,
			decodedTx.TxHash.Hex(),
			blockNum,
			blockHash,
			decodedTx.TxHash.Bytes(),
			decodedTx.From.Bytes(),
			decodedTx.To.Bytes(),
			decodedTx.To.Bytes(),
			decodedTx.MethodName,
			txParamsJSON,
			valueStr,
		)
		if err != nil {
			return fmt.Errorf("failed to insert transaction: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	log.Printf("Inserted %d transactions", len(txs))
	return nil
}

func (db *DB) UpdateEventsBatch(ctx context.Context, blockNum int64, eventsByTxHash map[string][]*utils.DecodedEvent) error {
	if len(eventsByTxHash) == 0 {
		return nil
	}

	// Process each tx in its own transaction so one failure doesn't poison the batch
	for txHashStr, events := range eventsByTxHash {
		if len(events) == 0 {
			continue
		}

		// Sanitize events before marshaling
		sanitizedEvents := make([]*utils.DecodedEvent, len(events))
		for i, ev := range events {
			sanitizedEvents[i] = utils.SanitizeEvent(ev)
		}

		eventsJSON, err := json.Marshal(sanitizedEvents)
		if err != nil {
			return fmt.Errorf("failed to marshal events: %w", err)
		}

		// Validate JSON
		if !json.Valid(eventsJSON) {
			return fmt.Errorf("invalid JSON for tx %s", txHashStr)
		}

		// Validate JSON by parsing and re-marshaling to ensure it's valid
		var testJSON interface{}
		if err := json.Unmarshal(eventsJSON, &testJSON); err != nil {
			log.Printf("Warning: JSON is invalid for tx %s: %v", txHashStr, err)
			// Skip this transaction
			continue
		}

		// Re-marshal to ensure clean, valid JSON
		cleanJSON, err := json.Marshal(testJSON)
		if err != nil {
			log.Printf("Warning: failed to re-marshal JSON for tx %s: %v", txHashStr, err)
			continue
		}

		// Final sanitization: replace control chars (\u0000-\u001F) with space
		jsonStr := utils.SanitizeJSONString(string(cleanJSON))

		first := events[0]
		txHash := first.TxHash.Bytes()
		blockHash := first.BlockHash.Bytes()
		contract := first.ContractAddress.Bytes()

		// Use parameterized query - lib/pq will handle the JSONB conversion
		// Try using json.RawMessage type hint or ensure proper escaping
		tx, err := db.conn.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("failed to begin transaction: %w", err)
		}

		query := `
			INSERT INTO actions (block_num, block_hash, tx_hash, tx_index, from_addr, to_addr, contract, events)
			VALUES ($1, $2, $3, 0, $4, $4, $4, $5::jsonb)
			ON CONFLICT (block_num, tx_hash) DO UPDATE SET
				events = COALESCE(actions.events, '[]'::jsonb) || EXCLUDED.events
		`
		_, err = tx.ExecContext(ctx, query, blockNum, blockHash, txHash, contract, jsonStr)
		if err != nil {
			if err := tx.Rollback(); err != nil {
				log.Printf("Failed to rollback transaction: %v", err)
			}
			log.Printf("Skipping transaction %s (block %d) due to error: %v", txHashStr, blockNum, err)
			continue
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("failed to commit transaction: %w", err)
		}
	}

	log.Printf("Updated events for %d transactions", len(eventsByTxHash))
	return nil
}

func (db *DB) GetLastIndexedBlock(ctx context.Context, chainID string) (int64, error) {
	query := `SELECT last_indexed FROM indexing_state WHERE chain_id = $1`
	var last int64
	err := db.conn.QueryRowContext(ctx, query, chainID).Scan(&last)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("failed to get last indexed block: %w", err)
	}
	return last, nil
}

func (db *DB) SetLastIndexedBlock(ctx context.Context, chainID string, blockNum int64) error {
	query := `
		INSERT INTO indexing_state (chain_id, last_indexed, head_seen, reorg_depth)
		VALUES ($1, $2, $2, 12)
		ON CONFLICT (chain_id) DO UPDATE SET
			last_indexed = EXCLUDED.last_indexed,
			head_seen = GREATEST(indexing_state.head_seen, EXCLUDED.head_seen),
			updated_at = now()
	`
	_, err := db.conn.ExecContext(ctx, query, chainID, blockNum)
	if err != nil {
		return fmt.Errorf("failed to set last indexed block: %w", err)
	}
	return nil
}

// GetStats returns basic statistics about indexed data.
func (db *DB) GetStats(ctx context.Context) (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	// Total actions
	var totalActions int64
	err := db.conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM actions`).Scan(&totalActions)
	if err != nil {
		return nil, fmt.Errorf("failed to get total actions: %w", err)
	}
	stats["total_actions"] = totalActions

	// Block range
	var minBlock, maxBlock sql.NullInt64
	err = db.conn.QueryRowContext(ctx, `SELECT MIN(block_num), MAX(block_num) FROM actions`).Scan(&minBlock, &maxBlock)
	if err != nil {
		return nil, fmt.Errorf("failed to get block range: %w", err)
	}
	if minBlock.Valid {
		stats["min_block"] = minBlock.Int64
		stats["max_block"] = maxBlock.Int64
	}

	// Indexing state
	var lastIndexed sql.NullInt64
	err = db.conn.QueryRowContext(ctx, `SELECT MAX(last_indexed) FROM indexing_state`).Scan(&lastIndexed)
	if err != nil {
		return nil, fmt.Errorf("failed to get last indexed: %w", err)
	}
	if lastIndexed.Valid {
		stats["last_indexed"] = lastIndexed.Int64
	}

	// Unique transactions
	var uniqueTxs int64
	err = db.conn.QueryRowContext(ctx, `SELECT COUNT(DISTINCT tx_hash) FROM actions`).Scan(&uniqueTxs)
	if err != nil {
		return nil, fmt.Errorf("failed to get unique transactions: %w", err)
	}
	stats["unique_transactions"] = uniqueTxs

	// Total blocks
	var totalBlocks int64
	err = db.conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM blocks`).Scan(&totalBlocks)
	if err != nil {
		return nil, fmt.Errorf("failed to get total blocks: %w", err)
	}
	stats["total_blocks"] = totalBlocks
	return stats, nil
}
