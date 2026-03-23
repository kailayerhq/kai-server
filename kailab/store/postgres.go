// Package store provides Postgres-backed storage for Kailab.
package store

import (
	"context"
	"database/sql"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"kai-core/cas"
	_ "github.com/lib/pq"
	"kailab/blobstore"
)

//go:embed schema_pg.sql
var schemaPgSQL string

// PgDB wraps a Postgres connection for Kailab storage.
type PgDB struct {
	conn   *sql.DB
	repoID string
}

// OpenRepoPgDB connects to Postgres and ensures the repo exists in the repos table.
// connStr is a Postgres connection string, tenant and repo identify the repository.
func OpenRepoPgDB(connStr, tenant, repo string) (*PgDB, error) {
	conn, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("opening postgres: %w", err)
	}

	if err := conn.Ping(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("pinging postgres: %w", err)
	}

	// Apply schema
	if _, err := conn.Exec(schemaPgSQL); err != nil {
		conn.Close()
		return nil, fmt.Errorf("applying schema: %w", err)
	}

	// Ensure repo exists; generate a stable repo ID from tenant+repo
	repoID := tenant + "/" + repo
	ts := cas.NowMs()
	_, err = conn.Exec(
		`INSERT INTO repos (id, tenant, name, created_at) VALUES ($1, $2, $3, $4) ON CONFLICT DO NOTHING`,
		repoID, tenant, repo, ts,
	)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("ensuring repo exists: %w", err)
	}

	return &PgDB{conn: conn, repoID: repoID}, nil
}

// OpenPg opens a Postgres database with the given connection string and repoID.
func OpenPg(connStr, repoID string) (*PgDB, error) {
	conn, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("opening postgres: %w", err)
	}

	if err := conn.Ping(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("pinging postgres: %w", err)
	}

	// Apply schema
	if _, err := conn.Exec(schemaPgSQL); err != nil {
		conn.Close()
		return nil, fmt.Errorf("applying schema: %w", err)
	}

	return &PgDB{conn: conn, repoID: repoID}, nil
}

// Conn returns the underlying *sql.DB connection.
func (db *PgDB) Conn() *sql.DB {
	return db.conn
}

// RepoID returns the repo ID for this database.
func (db *PgDB) RepoID() string {
	return db.repoID
}

// Close closes the database connection.
func (db *PgDB) Close() error {
	return db.conn.Close()
}

// BeginTx starts a new transaction.
func (db *PgDB) BeginTx() (*sql.Tx, error) {
	return db.conn.Begin()
}

// ----- Segments -----

// InsertSegment stores a new segment blob.
func (db *PgDB) InsertSegment(tx *sql.Tx, checksum []byte, blob []byte) (int64, error) {
	ts := cas.NowMs()
	var id int64
	err := tx.QueryRow(
		`INSERT INTO segments (repo_id, ts, checksum, size, blob) VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		db.repoID, ts, checksum, len(blob), blob,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("inserting segment: %w", err)
	}
	return id, nil
}

// GetSegmentBlob retrieves a segment's blob by ID.
func (db *PgDB) GetSegmentBlob(segmentID int64) ([]byte, error) {
	var blob []byte
	err := db.conn.QueryRow(
		`SELECT blob FROM segments WHERE id = $1 AND repo_id = $2`, segmentID, db.repoID,
	).Scan(&blob)
	if err == sql.ErrNoRows {
		return nil, ErrSegmentNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("querying segment: %w", err)
	}
	return blob, nil
}

// ----- Objects -----

// InsertObject records an object's location within a segment.
// Uses ON CONFLICT DO NOTHING for idempotence.
func (db *PgDB) InsertObject(tx *sql.Tx, digest []byte, segmentID, off, length int64, kind string) error {
	ts := cas.NowMs()
	_, err := tx.Exec(
		`INSERT INTO objects (repo_id, digest, segment_id, off, len, kind, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7) ON CONFLICT DO NOTHING`,
		db.repoID, digest, segmentID, off, length, kind, ts,
	)
	if err != nil {
		return fmt.Errorf("inserting object: %w", err)
	}
	return nil
}

// GetObject retrieves object metadata by digest.
func (db *PgDB) GetObject(digest []byte) (*ObjectInfo, error) {
	var info ObjectInfo
	err := db.conn.QueryRow(
		`SELECT digest, segment_id, off, len, kind, created_at FROM objects WHERE digest = $1 AND repo_id = $2`,
		digest, db.repoID,
	).Scan(&info.Digest, &info.SegmentID, &info.Off, &info.Len, &info.Kind, &info.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrObjectNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("querying object: %w", err)
	}
	return &info, nil
}

// HasObject checks if an object exists by digest.
func (db *PgDB) HasObject(digest []byte) (bool, error) {
	var count int
	err := db.conn.QueryRow(
		`SELECT COUNT(*) FROM objects WHERE digest = $1 AND repo_id = $2`, digest, db.repoID,
	).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("checking object: %w", err)
	}
	return count > 0, nil
}

// HasObjects checks which digests exist. Returns a set of existing digests (hex-encoded).
func (db *PgDB) HasObjects(digests [][]byte) (map[string]bool, error) {
	if len(digests) == 0 {
		return make(map[string]bool), nil
	}

	result := make(map[string]bool)

	// Process in batches to avoid query size limits
	batchSize := 500
	for i := 0; i < len(digests); i += batchSize {
		end := i + batchSize
		if end > len(digests) {
			end = len(digests)
		}
		batch := digests[i:end]

		placeholders := make([]string, len(batch))
		args := make([]interface{}, len(batch)+1)
		args[0] = db.repoID
		for j, d := range batch {
			placeholders[j] = fmt.Sprintf("$%d", j+2)
			args[j+1] = d
		}

		query := fmt.Sprintf(
			`SELECT digest FROM objects WHERE repo_id = $1 AND digest IN (%s)`,
			strings.Join(placeholders, ","),
		)

		rows, err := db.conn.Query(query, args...)
		if err != nil {
			return nil, fmt.Errorf("querying objects: %w", err)
		}

		for rows.Next() {
			var d []byte
			if err := rows.Scan(&d); err != nil {
				rows.Close()
				return nil, fmt.Errorf("scanning object: %w", err)
			}
			result[hex.EncodeToString(d)] = true
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("iterating objects: %w", err)
		}
	}

	return result, nil
}

// ReadObjectContent reads the actual content of an object by fetching from its segment.
func (db *PgDB) ReadObjectContent(digest []byte) ([]byte, error) {
	info, err := db.GetObject(digest)
	if err != nil {
		return nil, err
	}

	blob, err := db.GetSegmentBlob(info.SegmentID)
	if err != nil {
		return nil, err
	}

	if info.Off+info.Len > int64(len(blob)) {
		return nil, fmt.Errorf("object extends beyond segment bounds")
	}

	return blob[info.Off : info.Off+info.Len], nil
}

// ----- Refs -----

// GetRef retrieves a ref by name.
func (db *PgDB) GetRef(name string) (*Ref, error) {
	var ref Ref
	err := db.conn.QueryRow(
		`SELECT name, target, updated_at, actor, push_id FROM refs WHERE name = $1 AND repo_id = $2`,
		name, db.repoID,
	).Scan(&ref.Name, &ref.Target, &ref.UpdatedAt, &ref.Actor, &ref.PushID)
	if err == sql.ErrNoRows {
		return nil, ErrRefNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("querying ref: %w", err)
	}
	return &ref, nil
}

// ListRefs returns all refs, optionally filtered by prefix.
func (db *PgDB) ListRefs(prefix string) ([]*Ref, error) {
	var rows *sql.Rows
	var err error
	if prefix == "" {
		rows, err = db.conn.Query(
			`SELECT name, target, updated_at, actor, push_id FROM refs WHERE repo_id = $1 ORDER BY name`,
			db.repoID,
		)
	} else {
		rows, err = db.conn.Query(
			`SELECT name, target, updated_at, actor, push_id FROM refs WHERE repo_id = $1 AND name LIKE $2 ORDER BY name`,
			db.repoID, prefix+"%",
		)
	}
	if err != nil {
		return nil, fmt.Errorf("querying refs: %w", err)
	}
	defer rows.Close()

	var refs []*Ref
	for rows.Next() {
		var ref Ref
		if err := rows.Scan(&ref.Name, &ref.Target, &ref.UpdatedAt, &ref.Actor, &ref.PushID); err != nil {
			return nil, fmt.Errorf("scanning ref: %w", err)
		}
		refs = append(refs, &ref)
	}
	return refs, rows.Err()
}

// SetRefFF updates a ref with fast-forward check.
// If old is nil, the ref must not exist.
// If old is non-nil, the current target must match old.
func (db *PgDB) SetRefFF(tx *sql.Tx, name string, old, new []byte, actor, pushID string) error {
	ts := cas.NowMs()

	// Get current value
	var currentTarget []byte
	err := tx.QueryRow(`SELECT target FROM refs WHERE name = $1 AND repo_id = $2`, name, db.repoID).Scan(&currentTarget)
	if err == sql.ErrNoRows {
		currentTarget = nil
	} else if err != nil {
		return fmt.Errorf("checking current ref: %w", err)
	}

	// Verify fast-forward
	if old == nil && currentTarget != nil {
		return ErrRefMismatch
	}
	if old != nil {
		if currentTarget == nil {
			return ErrRefMismatch
		}
		if !bytesEqual(old, currentTarget) {
			return ErrRefMismatch
		}
	}

	// Get the last ref_history entry for this ref to chain
	var parentID []byte
	err = tx.QueryRow(
		`SELECT id FROM ref_history WHERE ref = $1 AND repo_id = $2 ORDER BY seq DESC LIMIT 1`,
		name, db.repoID,
	).Scan(&parentID)
	if err == sql.ErrNoRows {
		parentID = nil
	} else if err != nil {
		return fmt.Errorf("getting parent history: %w", err)
	}

	// Upsert ref
	_, err = tx.Exec(
		`INSERT INTO refs (repo_id, name, target, updated_at, actor, push_id)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT(repo_id, name) DO UPDATE SET target=excluded.target, updated_at=excluded.updated_at, actor=excluded.actor, push_id=excluded.push_id`,
		db.repoID, name, new, ts, actor, pushID,
	)
	if err != nil {
		return fmt.Errorf("upserting ref: %w", err)
	}

	// Append to ref_history
	historyEntry := map[string]interface{}{
		"time":   ts,
		"actor":  actor,
		"ref":    name,
		"old":    hex.EncodeToString(old),
		"new":    hex.EncodeToString(new),
		"pushId": pushID,
	}
	if parentID != nil {
		historyEntry["parent"] = hex.EncodeToString(parentID)
	}

	entryJSON, err := json.Marshal(historyEntry)
	if err != nil {
		return fmt.Errorf("marshaling history entry: %w", err)
	}
	entryID := cas.Blake3Hash(entryJSON)

	_, err = tx.Exec(
		`INSERT INTO ref_history (repo_id, id, parent, time, actor, ref, old, new, meta) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		db.repoID, entryID, parentID, ts, actor, name, old, new, string(entryJSON),
	)
	if err != nil {
		return fmt.Errorf("inserting ref history: %w", err)
	}

	return nil
}

// ForceSetRef updates a ref without fast-forward check.
func (db *PgDB) ForceSetRef(tx *sql.Tx, name string, new []byte, actor, pushID string) error {
	ts := cas.NowMs()

	// Get current target for history
	var currentTarget []byte
	err := tx.QueryRow(`SELECT target FROM refs WHERE name = $1 AND repo_id = $2`, name, db.repoID).Scan(&currentTarget)
	if err == sql.ErrNoRows {
		currentTarget = nil
	} else if err != nil {
		return fmt.Errorf("checking current ref: %w", err)
	}

	// Get the last ref_history entry to chain
	var parentID []byte
	err = tx.QueryRow(
		`SELECT id FROM ref_history WHERE ref = $1 AND repo_id = $2 ORDER BY seq DESC LIMIT 1`,
		name, db.repoID,
	).Scan(&parentID)
	if err == sql.ErrNoRows {
		parentID = nil
	} else if err != nil {
		return fmt.Errorf("getting parent history: %w", err)
	}

	// Upsert ref
	_, err = tx.Exec(
		`INSERT INTO refs (repo_id, name, target, updated_at, actor, push_id)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT(repo_id, name) DO UPDATE SET target=excluded.target, updated_at=excluded.updated_at, actor=excluded.actor, push_id=excluded.push_id`,
		db.repoID, name, new, ts, actor, pushID,
	)
	if err != nil {
		return fmt.Errorf("upserting ref: %w", err)
	}

	// Append to ref_history (marked as force)
	historyEntry := map[string]interface{}{
		"time":   ts,
		"actor":  actor,
		"ref":    name,
		"old":    hex.EncodeToString(currentTarget),
		"new":    hex.EncodeToString(new),
		"pushId": pushID,
		"force":  true,
	}
	if parentID != nil {
		historyEntry["parent"] = hex.EncodeToString(parentID)
	}

	entryJSON, err := json.Marshal(historyEntry)
	if err != nil {
		return fmt.Errorf("marshaling history entry: %w", err)
	}
	entryID := cas.Blake3Hash(entryJSON)

	_, err = tx.Exec(
		`INSERT INTO ref_history (repo_id, id, parent, time, actor, ref, old, new, meta) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		db.repoID, entryID, parentID, ts, actor, name, currentTarget, new, string(entryJSON),
	)
	if err != nil {
		return fmt.Errorf("inserting ref history: %w", err)
	}

	return nil
}

// DeleteRef deletes a ref and appends a deletion entry to ref_history.
// If old is non-nil, it must match the current target.
func (db *PgDB) DeleteRef(tx *sql.Tx, name string, old []byte, actor, pushID string) error {
	ts := cas.NowMs()

	var currentTarget []byte
	err := tx.QueryRow(`SELECT target FROM refs WHERE name = $1 AND repo_id = $2`, name, db.repoID).Scan(&currentTarget)
	if err == sql.ErrNoRows {
		return ErrRefNotFound
	}
	if err != nil {
		return fmt.Errorf("checking current ref: %w", err)
	}
	if old != nil && !bytesEqual(old, currentTarget) {
		return ErrRefMismatch
	}

	var parentID []byte
	err = tx.QueryRow(
		`SELECT id FROM ref_history WHERE ref = $1 AND repo_id = $2 ORDER BY seq DESC LIMIT 1`,
		name, db.repoID,
	).Scan(&parentID)
	if err == sql.ErrNoRows {
		parentID = nil
	} else if err != nil {
		return fmt.Errorf("getting parent history: %w", err)
	}

	if _, err := tx.Exec(`DELETE FROM refs WHERE name = $1 AND repo_id = $2`, name, db.repoID); err != nil {
		return fmt.Errorf("deleting ref: %w", err)
	}

	historyEntry := map[string]interface{}{
		"time":   ts,
		"actor":  actor,
		"ref":    name,
		"old":    hex.EncodeToString(currentTarget),
		"new":    "",
		"pushId": pushID,
		"delete": true,
	}
	if parentID != nil {
		historyEntry["parent"] = hex.EncodeToString(parentID)
	}

	entryJSON, err := json.Marshal(historyEntry)
	if err != nil {
		return fmt.Errorf("marshaling history entry: %w", err)
	}
	entryID := cas.Blake3Hash(entryJSON)
	emptyTarget := []byte{}

	_, err = tx.Exec(
		`INSERT INTO ref_history (repo_id, id, parent, time, actor, ref, old, new, meta) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		db.repoID, entryID, parentID, ts, actor, name, currentTarget, emptyTarget, string(entryJSON),
	)
	if err != nil {
		return fmt.Errorf("inserting ref history: %w", err)
	}

	return nil
}

// ----- Ref History -----

// GetRefHistory retrieves ref history entries.
func (db *PgDB) GetRefHistory(refFilter string, afterSeq int64, limit int) ([]*RefHistoryEntry, error) {
	var rows *sql.Rows
	var err error

	if limit <= 0 {
		limit = 100
	}

	if refFilter == "" {
		rows, err = db.conn.Query(
			`SELECT seq, id, parent, time, actor, ref, old, new, meta
			 FROM ref_history WHERE repo_id = $1 AND seq > $2 ORDER BY seq ASC LIMIT $3`,
			db.repoID, afterSeq, limit,
		)
	} else {
		rows, err = db.conn.Query(
			`SELECT seq, id, parent, time, actor, ref, old, new, meta
			 FROM ref_history WHERE repo_id = $1 AND ref = $2 AND seq > $3 ORDER BY seq ASC LIMIT $4`,
			db.repoID, refFilter, afterSeq, limit,
		)
	}
	if err != nil {
		return nil, fmt.Errorf("querying ref history: %w", err)
	}
	defer rows.Close()

	var entries []*RefHistoryEntry
	for rows.Next() {
		var e RefHistoryEntry
		if err := rows.Scan(&e.Seq, &e.ID, &e.Parent, &e.Time, &e.Actor, &e.Ref, &e.Old, &e.New, &e.Meta); err != nil {
			return nil, fmt.Errorf("scanning ref history: %w", err)
		}
		entries = append(entries, &e)
	}
	return entries, rows.Err()
}

// GetLogHead returns the ID of the most recent ref_history entry.
func (db *PgDB) GetLogHead() ([]byte, error) {
	var id []byte
	err := db.conn.QueryRow(
		`SELECT id FROM ref_history WHERE repo_id = $1 ORDER BY seq DESC LIMIT 1`,
		db.repoID,
	).Scan(&id)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("querying log head: %w", err)
	}
	return id, nil
}

// ----- Node Publish -----

// RecordNodePublish records a new node announcement.
func (db *PgDB) RecordNodePublish(tx *sql.Tx, nodeID []byte, kind, actor string) error {
	ts := cas.NowMs()

	// Get last entry for chaining
	var parentID []byte
	err := tx.QueryRow(
		`SELECT id FROM node_publish WHERE repo_id = $1 ORDER BY seq DESC LIMIT 1`,
		db.repoID,
	).Scan(&parentID)
	if err == sql.ErrNoRows {
		parentID = nil
	} else if err != nil {
		return fmt.Errorf("getting parent publish: %w", err)
	}

	entry := map[string]interface{}{
		"time":   ts,
		"actor":  actor,
		"nodeId": hex.EncodeToString(nodeID),
		"kind":   kind,
	}
	if parentID != nil {
		entry["parent"] = hex.EncodeToString(parentID)
	}

	entryJSON, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshaling publish entry: %w", err)
	}
	entryID := cas.Blake3Hash(entryJSON)

	_, err = tx.Exec(
		`INSERT INTO node_publish (repo_id, id, parent, time, actor, node_id, kind) VALUES ($1, $2, $3, $4, $5, $6, $7) ON CONFLICT DO NOTHING`,
		db.repoID, entryID, parentID, ts, actor, nodeID, kind,
	)
	if err != nil {
		return fmt.Errorf("inserting node publish: %w", err)
	}

	return nil
}

// ----- Enrich Queue -----

// EnqueueForEnrichment adds a node to the enrichment queue.
func (db *PgDB) EnqueueForEnrichment(tx *sql.Tx, nodeID []byte, kind string) error {
	ts := cas.NowMs()
	_, err := tx.Exec(
		`INSERT INTO enrich_queue (repo_id, node_id, kind, status, created_at) VALUES ($1, $2, $3, 'pending', $4)`,
		db.repoID, nodeID, kind, ts,
	)
	if err != nil {
		return fmt.Errorf("enqueueing for enrichment: %w", err)
	}
	return nil
}

// ClaimEnrichmentItem atomically claims the next pending item.
func (db *PgDB) ClaimEnrichmentItem() (*EnrichQueueItem, error) {
	tx, err := db.conn.Begin()
	if err != nil {
		return nil, fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback()

	var item EnrichQueueItem
	err = tx.QueryRow(
		`SELECT id, node_id, kind, status, created_at FROM enrich_queue
		 WHERE status = 'pending' AND repo_id = $1
		 ORDER BY id ASC LIMIT 1 FOR UPDATE SKIP LOCKED`,
		db.repoID,
	).Scan(&item.ID, &item.NodeID, &item.Kind, &item.Status, &item.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("querying queue: %w", err)
	}

	ts := cas.NowMs()
	_, err = tx.Exec(
		`UPDATE enrich_queue SET status = 'processing', started_at = $1 WHERE id = $2 AND repo_id = $3`,
		ts, item.ID, db.repoID,
	)
	if err != nil {
		return nil, fmt.Errorf("updating queue item: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("committing claim: %w", err)
	}

	item.Status = "processing"
	item.StartedAt = &ts
	return &item, nil
}

// CompleteEnrichmentItem marks an item as done or failed.
func (db *PgDB) CompleteEnrichmentItem(id int64, errMsg string) error {
	ts := cas.NowMs()
	status := "done"
	var errPtr *string
	if errMsg != "" {
		status = "failed"
		errPtr = &errMsg
	}

	_, err := db.conn.Exec(
		`UPDATE enrich_queue SET status = $1, finished_at = $2, error = $3 WHERE id = $4 AND repo_id = $5`,
		status, ts, errPtr, id, db.repoID,
	)
	if err != nil {
		return fmt.Errorf("completing enrichment: %w", err)
	}
	return nil
}

// ----- Edges -----

// InsertEdge inserts a single edge, ignoring duplicates.
func (db *PgDB) InsertEdge(tx *sql.Tx, src []byte, edgeType string, dst []byte, at []byte) error {
	ts := cas.NowMs()
	_, err := tx.Exec(
		`INSERT INTO edges (repo_id, src, type, dst, at, created_at) VALUES ($1, $2, $3, $4, $5, $6) ON CONFLICT DO NOTHING`,
		db.repoID, src, edgeType, dst, at, ts,
	)
	if err != nil {
		return fmt.Errorf("inserting edge: %w", err)
	}
	return nil
}

// InsertEdgesBatch inserts multiple edges efficiently.
func (db *PgDB) InsertEdgesBatch(tx *sql.Tx, edges []Edge) error {
	if len(edges) == 0 {
		return nil
	}

	ts := cas.NowMs()
	stmt, err := tx.Prepare(
		`INSERT INTO edges (repo_id, src, type, dst, at, created_at) VALUES ($1, $2, $3, $4, $5, $6) ON CONFLICT DO NOTHING`,
	)
	if err != nil {
		return fmt.Errorf("preparing edge insert: %w", err)
	}
	defer stmt.Close()

	for _, e := range edges {
		if _, err := stmt.Exec(db.repoID, e.Src, e.Type, e.Dst, e.At, ts); err != nil {
			return fmt.Errorf("inserting edge: %w", err)
		}
	}
	return nil
}

// GetEdgesFrom returns all edges from a source node.
func (db *PgDB) GetEdgesFrom(src []byte, edgeType string) ([]Edge, error) {
	query := `SELECT src, type, dst, at FROM edges WHERE src = $1 AND repo_id = $2`
	args := []interface{}{src, db.repoID}
	if edgeType != "" {
		query += ` AND type = $3`
		args = append(args, edgeType)
	}

	rows, err := db.conn.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying edges: %w", err)
	}
	defer rows.Close()

	var edges []Edge
	for rows.Next() {
		var e Edge
		if err := rows.Scan(&e.Src, &e.Type, &e.Dst, &e.At); err != nil {
			return nil, fmt.Errorf("scanning edge: %w", err)
		}
		edges = append(edges, e)
	}
	return edges, rows.Err()
}

// GetEdgesTo returns all edges pointing to a destination node.
func (db *PgDB) GetEdgesTo(dst []byte, edgeType string) ([]Edge, error) {
	query := `SELECT src, type, dst, at FROM edges WHERE dst = $1 AND repo_id = $2`
	args := []interface{}{dst, db.repoID}
	if edgeType != "" {
		query += ` AND type = $3`
		args = append(args, edgeType)
	}

	rows, err := db.conn.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying edges: %w", err)
	}
	defer rows.Close()

	var edges []Edge
	for rows.Next() {
		var e Edge
		if err := rows.Scan(&e.Src, &e.Type, &e.Dst, &e.At); err != nil {
			return nil, fmt.Errorf("scanning edge: %w", err)
		}
		edges = append(edges, e)
	}
	return edges, rows.Err()
}

// GetEdgesBySnapshot returns all edges scoped to a specific snapshot.
func (db *PgDB) GetEdgesBySnapshot(at []byte, edgeType string) ([]Edge, error) {
	query := `SELECT src, type, dst, at FROM edges WHERE at = $1 AND repo_id = $2`
	args := []interface{}{at, db.repoID}
	if edgeType != "" {
		query += ` AND type = $3`
		args = append(args, edgeType)
	}

	rows, err := db.conn.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying edges: %w", err)
	}
	defer rows.Close()

	var edges []Edge
	for rows.Next() {
		var e Edge
		if err := rows.Scan(&e.Src, &e.Type, &e.Dst, &e.At); err != nil {
			return nil, fmt.Errorf("scanning edge: %w", err)
		}
		edges = append(edges, e)
	}
	return edges, rows.Err()
}

// ============================================================================
// Standalone functions for multi-repo Postgres support
// These functions take *sql.DB and repoID as parameters.
// ============================================================================

// PgBeginTx starts a new transaction on the given database.
func PgBeginTx(db *sql.DB) (*sql.Tx, error) {
	return db.Begin()
}

// PgHasObjects checks which digests exist (standalone function).
func PgHasObjects(db *sql.DB, repoID string, digests [][]byte) (map[string]bool, error) {
	if len(digests) == 0 {
		return make(map[string]bool), nil
	}

	result := make(map[string]bool)

	// Process in batches to avoid query size limits
	batchSize := 500
	for i := 0; i < len(digests); i += batchSize {
		end := i + batchSize
		if end > len(digests) {
			end = len(digests)
		}
		batch := digests[i:end]

		placeholders := make([]string, len(batch))
		args := make([]interface{}, len(batch)+1)
		args[0] = repoID
		for j, d := range batch {
			placeholders[j] = fmt.Sprintf("$%d", j+2)
			args[j+1] = d
		}

		query := fmt.Sprintf(
			`SELECT digest FROM objects WHERE repo_id = $1 AND digest IN (%s)`,
			strings.Join(placeholders, ","),
		)

		rows, err := db.Query(query, args...)
		if err != nil {
			return nil, fmt.Errorf("querying objects: %w", err)
		}

		for rows.Next() {
			var d []byte
			if err := rows.Scan(&d); err != nil {
				rows.Close()
				return nil, fmt.Errorf("scanning object: %w", err)
			}
			result[hex.EncodeToString(d)] = true
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("iterating objects: %w", err)
		}
	}

	return result, nil
}

// PgInsertSegmentTx stores a new segment blob (standalone function).
// If an external blob store is configured, the blob is stored externally
// and only metadata is kept in the database.
func PgInsertSegmentTx(tx *sql.Tx, repoID string, checksum []byte, blob []byte) (int64, error) {
	ts := cas.NowMs()
	var id int64

	// Always store blob inline in Postgres (safety net for GCS failures)
	err := tx.QueryRow(
		`INSERT INTO segments (repo_id, ts, checksum, size, blob) VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		repoID, ts, checksum, len(blob), blob,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("inserting segment: %w", err)
	}

	// Also write to GCS for fast range reads (best-effort — inline is the fallback)
	if blobstore.IsExternal() {
		if err := blobstore.Global().Put(context.Background(), repoID, blobstore.SegmentKey(id), blob); err != nil {
			log.Printf("WARNING: GCS write failed for segment %d (inline fallback available): %v", id, err)
		}
	}

	return id, nil
}

// PgInsertObjectTx records an object's location within a segment (standalone function).
func PgInsertObjectTx(tx *sql.Tx, repoID string, digest []byte, segmentID, off, length int64, kind string) error {
	ts := cas.NowMs()
	_, err := tx.Exec(
		`INSERT INTO objects (repo_id, digest, segment_id, off, len, kind, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7) ON CONFLICT DO NOTHING`,
		repoID, digest, segmentID, off, length, kind, ts,
	)
	if err != nil {
		return fmt.Errorf("inserting object: %w", err)
	}
	return nil
}

// PgGetObjectInfo retrieves object metadata by digest (standalone function).
func PgGetObjectInfo(db *sql.DB, repoID string, digest []byte) (*ObjectInfo, error) {
	var info ObjectInfo
	err := db.QueryRow(
		`SELECT digest, segment_id, off, len, kind, created_at FROM objects WHERE digest = $1 AND repo_id = $2`,
		digest, repoID,
	).Scan(&info.Digest, &info.SegmentID, &info.Off, &info.Len, &info.Kind, &info.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrObjectNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("querying object: %w", err)
	}
	return &info, nil
}

// PgGetSegmentBlobByID retrieves a segment's blob by ID (standalone function).
// Tries external blob store first, falls back to inline database blob.
func PgGetSegmentBlobByID(db *sql.DB, repoID string, segmentID int64) ([]byte, error) {
	if blobstore.IsExternal() {
		data, err := blobstore.Global().Get(context.Background(), repoID, blobstore.SegmentKey(segmentID))
		if err == nil && len(data) > 0 {
			return data, nil
		}
		// Fall through to inline if GCS read fails (data may be inline from before GCS was enabled)
	}

	// Read inline from database
	var blob []byte
	err := db.QueryRow(
		`SELECT blob FROM segments WHERE id = $1 AND repo_id = $2`, segmentID, repoID,
	).Scan(&blob)
	if err == sql.ErrNoRows {
		return nil, ErrSegmentNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("querying segment: %w", err)
	}
	return blob, nil
}

// PgReadObjectContentDB reads the actual content of an object (standalone function).
// Uses range reads from external blob store when available.
func PgReadObjectContentDB(db *sql.DB, repoID string, digest []byte) ([]byte, error) {
	info, err := PgGetObjectInfo(db, repoID, digest)
	if err != nil {
		return nil, err
	}

	// Use range read for external blob store — only fetch the needed bytes
	if blobstore.IsExternal() {
		data, err := blobstore.Global().GetRange(
			context.Background(), repoID,
			blobstore.SegmentKey(info.SegmentID),
			info.Off, info.Len,
		)
		if err == nil && len(data) > 0 {
			return data, nil
		}
		// Fall through to inline if GCS read fails
	}

	blob, err := PgGetSegmentBlobByID(db, repoID, info.SegmentID)
	if err != nil {
		return nil, err
	}

	if info.Off+info.Len > int64(len(blob)) {
		return nil, fmt.Errorf("object extends beyond segment bounds")
	}

	return blob[info.Off : info.Off+info.Len], nil
}

// PgBatchGetObjects retrieves multiple objects by digest in a single query,
// groups them by segment, and fetches each segment blob only once.
// Returns a map of hex-encoded digest → (content, kind).
func PgBatchGetObjects(db *sql.DB, repoID string, digests [][]byte) (map[string]ObjectContent, error) {
	if len(digests) == 0 {
		return nil, nil
	}

	// Step 1: Batch query the objects table
	placeholders := make([]string, len(digests))
	args := make([]interface{}, 0, len(digests)+1)
	args = append(args, repoID)
	for i, d := range digests {
		placeholders[i] = fmt.Sprintf("$%d", i+2)
		args = append(args, d)
	}

	query := fmt.Sprintf(
		`SELECT digest, segment_id, off, len, kind FROM objects WHERE repo_id = $1 AND digest IN (%s)`,
		strings.Join(placeholders, ","),
	)

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("batch querying objects: %w", err)
	}
	defer rows.Close()

	// Group by segment_id
	type objectRef struct {
		Digest    []byte
		Off       int64
		Len       int64
		Kind      string
		HexDigest string
	}
	segmentObjects := make(map[int64][]objectRef)
	for rows.Next() {
		var ref objectRef
		var segmentID int64
		if err := rows.Scan(&ref.Digest, &segmentID, &ref.Off, &ref.Len, &ref.Kind); err != nil {
			return nil, err
		}
		ref.HexDigest = hex.EncodeToString(ref.Digest)
		segmentObjects[segmentID] = append(segmentObjects[segmentID], ref)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Step 2: Fetch each segment once, extract all objects
	result := make(map[string]ObjectContent, len(digests))
	for segmentID, refs := range segmentObjects {
		// Try external blob store with range reads if available
		if blobstore.IsExternal() {
			allOk := true
			for _, ref := range refs {
				data, err := blobstore.Global().GetRange(
					context.Background(), repoID,
					blobstore.SegmentKey(segmentID),
					ref.Off, ref.Len,
				)
				if err != nil || len(data) == 0 {
					allOk = false
					break
				}
				result[ref.HexDigest] = ObjectContent{Data: data, Kind: ref.Kind}
			}
			if allOk {
				continue
			}
		}

		// Fall back to inline: fetch segment blob once
		blob, err := PgGetSegmentBlobByID(db, repoID, segmentID)
		if err != nil {
			continue
		}
		for _, ref := range refs {
			if ref.Off+ref.Len > int64(len(blob)) {
				continue
			}
			result[ref.HexDigest] = ObjectContent{
				Data: make([]byte, ref.Len),
				Kind: ref.Kind,
			}
			copy(result[ref.HexDigest].Data, blob[ref.Off:ref.Off+ref.Len])
		}
	}

	return result, nil
}

// ObjectContent holds extracted object data and kind.
type ObjectContent struct {
	Data []byte
	Kind string
}

// PgListRefs returns all refs, optionally filtered by prefix (standalone function).
func PgListRefs(db *sql.DB, repoID string, prefix string) ([]*Ref, error) {
	var rows *sql.Rows
	var err error
	if prefix == "" {
		rows, err = db.Query(
			`SELECT name, target, updated_at, actor, push_id FROM refs WHERE repo_id = $1 ORDER BY name`,
			repoID,
		)
	} else {
		rows, err = db.Query(
			`SELECT name, target, updated_at, actor, push_id FROM refs WHERE repo_id = $1 AND name LIKE $2 ORDER BY name`,
			repoID, prefix+"%",
		)
	}
	if err != nil {
		return nil, fmt.Errorf("querying refs: %w", err)
	}
	defer rows.Close()

	var refs []*Ref
	for rows.Next() {
		var ref Ref
		if err := rows.Scan(&ref.Name, &ref.Target, &ref.UpdatedAt, &ref.Actor, &ref.PushID); err != nil {
			return nil, fmt.Errorf("scanning ref: %w", err)
		}
		refs = append(refs, &ref)
	}
	return refs, rows.Err()
}

// PgGetRef retrieves a ref by name (standalone function).
func PgGetRef(db *sql.DB, repoID string, name string) (*Ref, error) {
	var ref Ref
	err := db.QueryRow(
		`SELECT name, target, updated_at, actor, push_id FROM refs WHERE name = $1 AND repo_id = $2`,
		name, repoID,
	).Scan(&ref.Name, &ref.Target, &ref.UpdatedAt, &ref.Actor, &ref.PushID)
	if err == sql.ErrNoRows {
		return nil, ErrRefNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("querying ref: %w", err)
	}
	return &ref, nil
}

// PgSetRefFF updates a ref with fast-forward check (standalone function).
func PgSetRefFF(db *sql.DB, tx *sql.Tx, repoID string, name string, old, new []byte, actor, pushID string) error {
	ts := cas.NowMs()

	// Get current value
	var currentTarget []byte
	err := tx.QueryRow(`SELECT target FROM refs WHERE name = $1 AND repo_id = $2`, name, repoID).Scan(&currentTarget)
	if err == sql.ErrNoRows {
		currentTarget = nil
	} else if err != nil {
		return fmt.Errorf("checking current ref: %w", err)
	}

	// Verify fast-forward
	if old == nil && currentTarget != nil {
		return ErrRefMismatch
	}
	if old != nil {
		if currentTarget == nil {
			return ErrRefMismatch
		}
		if !bytesEqual(old, currentTarget) {
			return ErrRefMismatch
		}
	}

	// Get the last ref_history entry for this ref to chain
	var parentID []byte
	err = tx.QueryRow(
		`SELECT id FROM ref_history WHERE ref = $1 AND repo_id = $2 ORDER BY seq DESC LIMIT 1`,
		name, repoID,
	).Scan(&parentID)
	if err == sql.ErrNoRows {
		parentID = nil
	} else if err != nil {
		return fmt.Errorf("getting parent history: %w", err)
	}

	// Upsert ref
	_, err = tx.Exec(
		`INSERT INTO refs (repo_id, name, target, updated_at, actor, push_id)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT(repo_id, name) DO UPDATE SET target=excluded.target, updated_at=excluded.updated_at, actor=excluded.actor, push_id=excluded.push_id`,
		repoID, name, new, ts, actor, pushID,
	)
	if err != nil {
		return fmt.Errorf("upserting ref: %w", err)
	}

	// Append to ref_history
	historyEntry := map[string]interface{}{
		"time":   ts,
		"actor":  actor,
		"ref":    name,
		"old":    hex.EncodeToString(old),
		"new":    hex.EncodeToString(new),
		"pushId": pushID,
	}
	if parentID != nil {
		historyEntry["parent"] = hex.EncodeToString(parentID)
	}

	entryJSON, err := json.Marshal(historyEntry)
	if err != nil {
		return fmt.Errorf("marshaling history entry: %w", err)
	}
	entryID := cas.Blake3Hash(entryJSON)

	_, err = tx.Exec(
		`INSERT INTO ref_history (repo_id, id, parent, time, actor, ref, old, new, meta) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		repoID, entryID, parentID, ts, actor, name, old, new, string(entryJSON),
	)
	if err != nil {
		return fmt.Errorf("inserting ref history: %w", err)
	}

	return nil
}

// PgForceSetRef updates a ref without fast-forward check (standalone function).
func PgForceSetRef(db *sql.DB, tx *sql.Tx, repoID string, name string, new []byte, actor, pushID string) error {
	ts := cas.NowMs()

	// Get current target for history
	var currentTarget []byte
	err := tx.QueryRow(`SELECT target FROM refs WHERE name = $1 AND repo_id = $2`, name, repoID).Scan(&currentTarget)
	if err == sql.ErrNoRows {
		currentTarget = nil
	} else if err != nil {
		return fmt.Errorf("checking current ref: %w", err)
	}

	// Get the last ref_history entry to chain
	var parentID []byte
	err = tx.QueryRow(
		`SELECT id FROM ref_history WHERE ref = $1 AND repo_id = $2 ORDER BY seq DESC LIMIT 1`,
		name, repoID,
	).Scan(&parentID)
	if err == sql.ErrNoRows {
		parentID = nil
	} else if err != nil {
		return fmt.Errorf("getting parent history: %w", err)
	}

	// Upsert ref
	_, err = tx.Exec(
		`INSERT INTO refs (repo_id, name, target, updated_at, actor, push_id)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT(repo_id, name) DO UPDATE SET target=excluded.target, updated_at=excluded.updated_at, actor=excluded.actor, push_id=excluded.push_id`,
		repoID, name, new, ts, actor, pushID,
	)
	if err != nil {
		return fmt.Errorf("upserting ref: %w", err)
	}

	// Append to ref_history (marked as force)
	historyEntry := map[string]interface{}{
		"time":   ts,
		"actor":  actor,
		"ref":    name,
		"old":    hex.EncodeToString(currentTarget),
		"new":    hex.EncodeToString(new),
		"pushId": pushID,
		"force":  true,
	}
	if parentID != nil {
		historyEntry["parent"] = hex.EncodeToString(parentID)
	}

	entryJSON, err := json.Marshal(historyEntry)
	if err != nil {
		return fmt.Errorf("marshaling history entry: %w", err)
	}
	entryID := cas.Blake3Hash(entryJSON)

	_, err = tx.Exec(
		`INSERT INTO ref_history (repo_id, id, parent, time, actor, ref, old, new, meta) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		repoID, entryID, parentID, ts, actor, name, currentTarget, new, string(entryJSON),
	)
	if err != nil {
		return fmt.Errorf("inserting ref history: %w", err)
	}

	return nil
}

// PgDeleteRef deletes a ref and appends a deletion entry to ref_history (standalone function).
func PgDeleteRef(db *sql.DB, tx *sql.Tx, repoID string, name string, old []byte, actor, pushID string) error {
	ts := cas.NowMs()

	var currentTarget []byte
	err := tx.QueryRow(`SELECT target FROM refs WHERE name = $1 AND repo_id = $2`, name, repoID).Scan(&currentTarget)
	if err == sql.ErrNoRows {
		return ErrRefNotFound
	}
	if err != nil {
		return fmt.Errorf("checking current ref: %w", err)
	}
	if old != nil && !bytesEqual(old, currentTarget) {
		return ErrRefMismatch
	}

	var parentID []byte
	err = tx.QueryRow(
		`SELECT id FROM ref_history WHERE ref = $1 AND repo_id = $2 ORDER BY seq DESC LIMIT 1`,
		name, repoID,
	).Scan(&parentID)
	if err == sql.ErrNoRows {
		parentID = nil
	} else if err != nil {
		return fmt.Errorf("getting parent history: %w", err)
	}

	if _, err := tx.Exec(`DELETE FROM refs WHERE name = $1 AND repo_id = $2`, name, repoID); err != nil {
		return fmt.Errorf("deleting ref: %w", err)
	}

	historyEntry := map[string]interface{}{
		"time":   ts,
		"actor":  actor,
		"ref":    name,
		"old":    hex.EncodeToString(currentTarget),
		"new":    "",
		"pushId": pushID,
		"delete": true,
	}
	if parentID != nil {
		historyEntry["parent"] = hex.EncodeToString(parentID)
	}

	entryJSON, err := json.Marshal(historyEntry)
	if err != nil {
		return fmt.Errorf("marshaling history entry: %w", err)
	}
	entryID := cas.Blake3Hash(entryJSON)
	emptyTarget := []byte{}

	_, err = tx.Exec(
		`INSERT INTO ref_history (repo_id, id, parent, time, actor, ref, old, new, meta) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		repoID, entryID, parentID, ts, actor, name, currentTarget, emptyTarget, string(entryJSON),
	)
	if err != nil {
		return fmt.Errorf("inserting ref history: %w", err)
	}

	return nil
}

// PgGetRefHistory retrieves ref history entries (standalone function).
func PgGetRefHistory(db *sql.DB, repoID string, refFilter string, afterSeq int64, limit int) ([]*RefHistoryEntry, error) {
	var rows *sql.Rows
	var err error

	if limit <= 0 {
		limit = 100
	}

	if refFilter == "" {
		rows, err = db.Query(
			`SELECT seq, id, parent, time, actor, ref, old, new, meta
			 FROM ref_history WHERE repo_id = $1 AND seq > $2 ORDER BY seq DESC LIMIT $3`,
			repoID, afterSeq, limit,
		)
	} else {
		rows, err = db.Query(
			`SELECT seq, id, parent, time, actor, ref, old, new, meta
			 FROM ref_history WHERE repo_id = $1 AND ref = $2 AND seq > $3 ORDER BY seq DESC LIMIT $4`,
			repoID, refFilter, afterSeq, limit,
		)
	}
	if err != nil {
		return nil, fmt.Errorf("querying ref history: %w", err)
	}
	defer rows.Close()

	var entries []*RefHistoryEntry
	for rows.Next() {
		var e RefHistoryEntry
		if err := rows.Scan(&e.Seq, &e.ID, &e.Parent, &e.Time, &e.Actor, &e.Ref, &e.Old, &e.New, &e.Meta); err != nil {
			return nil, fmt.Errorf("scanning ref history: %w", err)
		}
		entries = append(entries, &e)
	}
	return entries, rows.Err()
}

// PgGetLogHead returns the ID of the most recent ref_history entry (standalone function).
func PgGetLogHead(db *sql.DB, repoID string) ([]byte, error) {
	var id []byte
	err := db.QueryRow(
		`SELECT id FROM ref_history WHERE repo_id = $1 ORDER BY seq DESC LIMIT 1`,
		repoID,
	).Scan(&id)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("querying log head: %w", err)
	}
	return id, nil
}

// PgEnqueueForEnrichmentTx adds a node to the enrichment queue (standalone function).
func PgEnqueueForEnrichmentTx(tx *sql.Tx, repoID string, nodeID []byte, kind string) error {
	ts := cas.NowMs()
	_, err := tx.Exec(
		`INSERT INTO enrich_queue (repo_id, node_id, kind, status, created_at) VALUES ($1, $2, $3, 'pending', $4)`,
		repoID, nodeID, kind, ts,
	)
	if err != nil {
		return fmt.Errorf("enqueueing for enrichment: %w", err)
	}
	return nil
}

// ----- Standalone Edge Functions (for use with raw *sql.DB) -----

// PgInsertEdgesTx inserts multiple edges in a transaction (standalone function).
func PgInsertEdgesTx(tx *sql.Tx, repoID string, edges []Edge) error {
	if len(edges) == 0 {
		return nil
	}

	ts := cas.NowMs()
	stmt, err := tx.Prepare(
		`INSERT INTO edges (repo_id, src, type, dst, at, created_at) VALUES ($1, $2, $3, $4, $5, $6) ON CONFLICT DO NOTHING`,
	)
	if err != nil {
		return fmt.Errorf("preparing edge insert: %w", err)
	}
	defer stmt.Close()

	for _, e := range edges {
		if _, err := stmt.Exec(repoID, e.Src, e.Type, e.Dst, e.At, ts); err != nil {
			return fmt.Errorf("inserting edge: %w", err)
		}
	}
	return nil
}

// PgGetEdgesFromDB returns edges from a source node (standalone function).
func PgGetEdgesFromDB(db *sql.DB, repoID string, src []byte, edgeType string) ([]Edge, error) {
	query := `SELECT src, type, dst, at FROM edges WHERE src = $1 AND repo_id = $2`
	args := []interface{}{src, repoID}
	if edgeType != "" {
		query += ` AND type = $3`
		args = append(args, edgeType)
	}

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying edges: %w", err)
	}
	defer rows.Close()

	var edges []Edge
	for rows.Next() {
		var e Edge
		if err := rows.Scan(&e.Src, &e.Type, &e.Dst, &e.At); err != nil {
			return nil, fmt.Errorf("scanning edge: %w", err)
		}
		edges = append(edges, e)
	}
	return edges, rows.Err()
}

// PgGetEdgesToDB returns edges pointing to a destination node (standalone function).
func PgGetEdgesToDB(db *sql.DB, repoID string, dst []byte, edgeType string) ([]Edge, error) {
	query := `SELECT src, type, dst, at FROM edges WHERE dst = $1 AND repo_id = $2`
	args := []interface{}{dst, repoID}
	if edgeType != "" {
		query += ` AND type = $3`
		args = append(args, edgeType)
	}

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying edges: %w", err)
	}
	defer rows.Close()

	var edges []Edge
	for rows.Next() {
		var e Edge
		if err := rows.Scan(&e.Src, &e.Type, &e.Dst, &e.At); err != nil {
			return nil, fmt.Errorf("scanning edge: %w", err)
		}
		edges = append(edges, e)
	}
	return edges, rows.Err()
}

// PgGetEdgesBySnapshotDB returns edges for a specific snapshot (standalone function).
func PgGetEdgesBySnapshotDB(db *sql.DB, repoID string, at []byte, edgeType string) ([]Edge, error) {
	query := `SELECT src, type, dst, at FROM edges WHERE at = $1 AND repo_id = $2`
	args := []interface{}{at, repoID}
	if edgeType != "" {
		query += ` AND type = $3`
		args = append(args, edgeType)
	}

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying edges: %w", err)
	}
	defer rows.Close()

	var edges []Edge
	for rows.Next() {
		var e Edge
		if err := rows.Scan(&e.Src, &e.Type, &e.Dst, &e.At); err != nil {
			return nil, fmt.Errorf("scanning edge: %w", err)
		}
		edges = append(edges, e)
	}
	return edges, rows.Err()
}

// PgSchemaSQL returns the embedded Postgres schema SQL.
func PgSchemaSQL() string {
	return schemaPgSQL
}
