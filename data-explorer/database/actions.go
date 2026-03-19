package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
)

// ActionRow is a single row from the actions table joined with blocks.
type ActionRow struct {
	ID        int64
	BlockNum  int64
	BlockTime time.Time
	TxHash    string          // "0x…" hex
	Method    string          // empty when undecoded
	Caller    string          // "0x…" hex  (from_addr)
	Contract  string          // "0x…" hex
	TxParams  json.RawMessage // full decoded input params
	Events    json.RawMessage // full decoded event array
	Value     string          // wei as decimal string
}

type ActionFilter struct {
	Method     string
	Caller     string    // hex address
	Contract   string    // hex address
	TxParamKey string    // arbitrary tx_params key
	TxParamVal string    // matches scalar or array element
	FromBlock  int64     // 0 = unset
	ToBlock    int64     // 0 = unset
	FromTime   time.Time // zero = unset
	ToTime     time.Time // zero = unset
}

// CursorVal is the (block_num, id) bookmark used for keyset pagination.
type CursorVal struct {
	BlockNum int64
	ID       int64
}

// ListActionsResult is the page of rows plus the next-page cursor.
type ListActionsResult struct {
	Rows       []ActionRow
	NextCursor *CursorVal // nil when this is the last page
}

// ListActions runs a filtered, cursor-paginated SELECT against the actions table.
func (db *DB) ListActions(ctx context.Context, f ActionFilter, after *CursorVal, limit int) (ListActionsResult, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}

	args := []any{}
	conds := []string{}

	// Keyset cursor – skip rows the caller has already seen.
	if after != nil {
		args = append(args, after.BlockNum, after.ID)
		conds = append(conds, fmt.Sprintf("(a.block_num, a.id) > ($%d, $%d)", len(args)-1, len(args)))
	}
	if f.Method != "" {
		args = append(args, f.Method)
		conds = append(conds, fmt.Sprintf("a.method = $%d", len(args)))
	}
	if f.Caller != "" {
		args = append(args, common.HexToAddress(f.Caller).Bytes())
		conds = append(conds, fmt.Sprintf("a.from_addr = $%d", len(args)))
	}
	if f.Contract != "" {
		args = append(args, common.HexToAddress(f.Contract).Bytes())
		conds = append(conds, fmt.Sprintf("a.contract = $%d", len(args)))
	}
	if f.TxParamKey != "" && f.TxParamVal != "" {

		args = append(args, f.TxParamKey, f.TxParamVal, f.TxParamKey, f.TxParamVal)
		ginCond := fmt.Sprintf(
			"(a.tx_params @> jsonb_build_object($%d::text, to_jsonb($%d::text)) OR a.tx_params @> jsonb_build_object($%d::text, jsonb_build_array(to_jsonb($%d::text))))",
			len(args)-3, len(args)-2, len(args)-1, len(args))

		escapedKey := strings.ReplaceAll(f.TxParamKey, "\"", "\"\"")
		path := fmt.Sprintf(`$."%s".** ? (@ == $val)`, escapedKey)
		var jsonb interface{}
		if err := json.Unmarshal([]byte(f.TxParamVal), &jsonb); err != nil {
			jsonb = f.TxParamVal
		}
		valJSON, _ := json.Marshal(jsonb)
		args = append(args, path, valJSON)

		jsonPathCond := fmt.Sprintf(
			"jsonb_path_exists(a.tx_params, $%d::jsonpath, jsonb_build_object('val', $%d::jsonb))",
			len(args)-1, len(args))

		// Combine both so either match succeeds.
		conds = append(conds, fmt.Sprintf("(%s OR %s)", ginCond, jsonPathCond))
	}
	if f.FromBlock > 0 {
		args = append(args, f.FromBlock)
		conds = append(conds, fmt.Sprintf("a.block_num >= $%d", len(args)))
	}
	if f.ToBlock > 0 {
		args = append(args, f.ToBlock)
		conds = append(conds, fmt.Sprintf("a.block_num <= $%d", len(args)))
	}
	if !f.FromTime.IsZero() {
		args = append(args, f.FromTime)
		conds = append(conds, fmt.Sprintf("b.timestamp >= $%d", len(args)))
	}
	if !f.ToTime.IsZero() {
		args = append(args, f.ToTime)
		conds = append(conds, fmt.Sprintf("b.timestamp <= $%d", len(args)))
	}

	where := ""
	if len(conds) > 0 {
		where = "WHERE " + strings.Join(conds, " AND ")
	}

	// Fetch one extra row to detect whether a next page exists.
	args = append(args, limit+1)
	query := fmt.Sprintf(`
		SELECT
			a.id,
			a.block_num,
			COALESCE(b.timestamp, now())            AS block_time,
			encode(a.tx_hash,   'hex')              AS tx_hash,
			COALESCE(a.method,  '')                 AS method,
			encode(a.from_addr, 'hex')              AS caller,
			encode(a.contract,  'hex')              AS contract,
			COALESCE(a.tx_params, '{}'::jsonb)      AS tx_params,
			COALESCE(a.events,   '[]'::jsonb)       AS events,
			COALESCE(a.value::text, '0')            AS value
		FROM actions a
		LEFT JOIN blocks b ON b.num = a.block_num
		%s
		ORDER BY a.block_num ASC, a.id ASC
		LIMIT $%d
	`, where, len(args))

	rows, err := db.conn.QueryContext(ctx, query, args...)
	if err != nil {
		return ListActionsResult{}, fmt.Errorf("ListActions: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var result []ActionRow
	for rows.Next() {
		var r ActionRow
		var txp, evs []byte
		if err := rows.Scan(
			&r.ID, &r.BlockNum, &r.BlockTime,
			&r.TxHash, &r.Method, &r.Caller, &r.Contract,
			&txp, &evs, &r.Value,
		); err != nil {
			return ListActionsResult{}, fmt.Errorf("ListActions scan: %w", err)
		}
		r.TxHash = "0x" + r.TxHash
		r.Caller = "0x" + r.Caller
		r.Contract = "0x" + r.Contract
		r.TxParams = json.RawMessage(txp)
		r.Events = json.RawMessage(evs)
		result = append(result, r)
	}
	if err := rows.Err(); err != nil {
		return ListActionsResult{}, fmt.Errorf("ListActions rows: %w", err)
	}

	var next *CursorVal
	if len(result) > limit {
		last := result[limit-1]
		next = &CursorVal{BlockNum: last.BlockNum, ID: last.ID}
		result = result[:limit]
	}

	return ListActionsResult{Rows: result, NextCursor: next}, nil
}

// GetActionByID returns one fully-detailed action by its composite PK (block_num, id).
func (db *DB) GetActionByID(ctx context.Context, blockNum, id int64) (*ActionRow, error) {
	query := `
		SELECT
			a.id,
			a.block_num,
			COALESCE(b.timestamp, now())            AS block_time,
			encode(a.tx_hash,   'hex')              AS tx_hash,
			COALESCE(a.method,  '')                 AS method,
			encode(a.from_addr, 'hex')              AS caller,
			encode(a.contract,  'hex')              AS contract,
			COALESCE(a.tx_params, '{}'::jsonb)      AS tx_params,
			COALESCE(a.events,   '[]'::jsonb)       AS events,
			COALESCE(a.value::text, '0')            AS value
		FROM actions a
		LEFT JOIN blocks b ON b.num = a.block_num
		WHERE a.block_num = $1 AND a.id = $2
		LIMIT 1
	`
	var r ActionRow
	var txp, evs []byte
	err := db.conn.QueryRowContext(ctx, query, blockNum, id).Scan(
		&r.ID, &r.BlockNum, &r.BlockTime,
		&r.TxHash, &r.Method, &r.Caller, &r.Contract,
		&txp, &evs, &r.Value,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("GetActionByID: %w", err)
	}
	r.TxHash = "0x" + r.TxHash
	r.Caller = "0x" + r.Caller
	r.Contract = "0x" + r.Contract
	r.TxParams = json.RawMessage(txp)
	r.Events = json.RawMessage(evs)
	return &r, nil
}

// GetDistinctMethods returns every unique decoded method name.
func (db *DB) GetDistinctMethods(ctx context.Context) ([]string, error) {
	rows, err := db.conn.QueryContext(ctx,
		`SELECT DISTINCT method FROM actions
		 WHERE method IS NOT NULL AND method <> ''
		 ORDER BY method`)
	if err != nil {
		return nil, fmt.Errorf("GetDistinctMethods: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var out []string
	for rows.Next() {
		var m string
		if err := rows.Scan(&m); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// ContractRow is one row from the contracts table.
type ContractRow struct {
	Address string `json:"address"`
	Name    string `json:"name"`
}

// GetContracts returns all known contracts ordered by name.
func (db *DB) GetContracts(ctx context.Context) ([]ContractRow, error) {
	rows, err := db.conn.QueryContext(ctx,
		`SELECT encode(address,'hex'), COALESCE(name,'') FROM contracts ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("GetContracts: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var out []ContractRow
	for rows.Next() {
		var c ContractRow
		if err := rows.Scan(&c.Address, &c.Name); err != nil {
			return nil, err
		}
		c.Address = "0x" + c.Address
		out = append(out, c)
	}
	return out, rows.Err()
}
