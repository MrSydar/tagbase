package db

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"mrsydar/tagbase/storage/internal/models"
)

// DB wraps pgxpool and provides typed queries.
type DB struct {
	pool *pgxpool.Pool
}

// New creates a new DB instance.
func New(pool *pgxpool.Pool) *DB {
	return &DB{pool: pool}
}

// Close closes the pool.
func (d *DB) Close() {
	d.pool.Close()
}

// Pool returns the underlying pool.
func (d *DB) Pool() *pgxpool.Pool {
	return d.pool
}

// CreateCollection inserts a collection.
func (d *DB) CreateCollection(ctx context.Context, name, dataType string) (*models.Collection, error) {
	var c models.Collection
	err := d.pool.QueryRow(ctx,
		`INSERT INTO collections (name, data_type) VALUES ($1, $2) RETURNING id, name, data_type, created_at`,
		name, dataType,
	).Scan(&c.ID, &c.Name, &c.DataType, &c.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("insert collection: %w", err)
	}
	return &c, nil
}

// GetCollectionByName fetches a collection by name.
func (d *DB) GetCollectionByName(ctx context.Context, name string) (*models.Collection, error) {
	var c models.Collection
	err := d.pool.QueryRow(ctx,
		`SELECT id, name, data_type, created_at FROM collections WHERE name = $1`,
		name,
	).Scan(&c.ID, &c.Name, &c.DataType, &c.CreatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("collection not found: %w", err)
		}
		return nil, fmt.Errorf("get collection: %w", err)
	}
	return &c, nil
}

// ListCollections returns all collections.
func (d *DB) ListCollections(ctx context.Context) ([]models.Collection, error) {
	rows, err := d.pool.Query(ctx,
		`SELECT id, name, data_type, created_at FROM collections ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list collections: %w", err)
	}
	defer rows.Close()

	var collections []models.Collection
	for rows.Next() {
		var c models.Collection
		if err := rows.Scan(&c.ID, &c.Name, &c.DataType, &c.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan collection: %w", err)
		}
		collections = append(collections, c)
	}
	return collections, rows.Err()
}

// GetCollectionPayloadKeys returns payload keys for all objects in a collection.
func (d *DB) GetCollectionPayloadKeys(ctx context.Context, collectionID string) ([]string, error) {
	rows, err := d.pool.Query(ctx,
		`SELECT payload_key FROM objects WHERE collection_id = $1`,
		collectionID,
	)
	if err != nil {
		return nil, fmt.Errorf("get collection payload keys: %w", err)
	}
	defer rows.Close()

	var keys []string
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, fmt.Errorf("scan payload key: %w", err)
		}
		keys = append(keys, key)
	}
	return keys, rows.Err()
}

// DeleteCollection deletes a collection by ID. Cascades to objects and tags via FK.
func (d *DB) DeleteCollection(ctx context.Context, collectionID string) error {
	_, err := d.pool.Exec(ctx,
		`DELETE FROM collections WHERE id = $1`,
		collectionID,
	)
	if err != nil {
		return fmt.Errorf("delete collection: %w", err)
	}
	return nil
}

// GetObjectByID fetches object metadata by ID, joined with collection name.
func (d *DB) GetObjectByID(ctx context.Context, id string) (*models.Object, error) {
	var o models.Object
	var expiresAt *time.Time
	err := d.pool.QueryRow(ctx,
		`SELECT o.id, o.collection_id, c.name, o.data_type, o.date, o.size_bytes, o.content_hash, o.created_at, o.expires_at, o.payload_key
		 FROM objects o JOIN collections c ON o.collection_id = c.id
		 WHERE o.id = $1 AND (o.expires_at IS NULL OR o.expires_at > NOW())`,
		id,
	).Scan(&o.ID, &o.CollectionID, &o.Collection, &o.DataType, &o.Date, &o.SizeBytes, &o.ContentHash, &o.CreatedAt, &expiresAt, &o.PayloadKey)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("object not found: %w", err)
		}
		return nil, fmt.Errorf("get object: %w", err)
	}
	o.ExpiresAt = expiresAt
	return &o, nil
}

// GetObjectByCollectionAndHash fetches an object by collection ID and content hash.
func (d *DB) GetObjectByCollectionAndHash(ctx context.Context, collectionID, hash string) (*models.Object, error) {
	var o models.Object
	var expiresAt *time.Time
	var collName string
	err := d.pool.QueryRow(ctx,
		`SELECT o.id, o.collection_id, c.name, o.data_type, o.date, o.size_bytes, o.content_hash, o.created_at, o.expires_at, o.payload_key
		 FROM objects o JOIN collections c ON o.collection_id = c.id
		 WHERE o.collection_id = $1 AND o.content_hash = $2 AND (o.expires_at IS NULL OR o.expires_at > NOW())`,
		collectionID, hash,
	).Scan(&o.ID, &o.CollectionID, &collName, &o.DataType, &o.Date, &o.SizeBytes, &o.ContentHash, &o.CreatedAt, &expiresAt, &o.PayloadKey)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("object not found: %w", err)
		}
		return nil, fmt.Errorf("get object by hash: %w", err)
	}
	o.Collection = collName
	o.ExpiresAt = expiresAt
	return &o, nil
}

// GetObjectByCollectionAndHashIncludingExpired fetches an object by collection ID and content hash, including expired rows.
func (d *DB) GetObjectByCollectionAndHashIncludingExpired(ctx context.Context, collectionID, hash string) (*models.Object, error) {
	var o models.Object
	var expiresAt *time.Time
	var collName string
	err := d.pool.QueryRow(ctx,
		`SELECT o.id, o.collection_id, c.name, o.data_type, o.date, o.size_bytes, o.content_hash, o.created_at, o.expires_at, o.payload_key
		 FROM objects o JOIN collections c ON o.collection_id = c.id
		 WHERE o.collection_id = $1 AND o.content_hash = $2`,
		collectionID, hash,
	).Scan(&o.ID, &o.CollectionID, &collName, &o.DataType, &o.Date, &o.SizeBytes, &o.ContentHash, &o.CreatedAt, &expiresAt, &o.PayloadKey)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("object not found: %w", err)
		}
		return nil, fmt.Errorf("get object by hash: %w", err)
	}
	o.Collection = collName
	o.ExpiresAt = expiresAt
	return &o, nil
}

// InsertObject inserts a new object.
func (d *DB) InsertObject(ctx context.Context, collectionID string, hash string, date time.Time, sizeBytes int64, dataType, payloadKey string, expiresAt *time.Time) (*models.Object, error) {
	var id string
	var createdAt time.Time
	err := d.pool.QueryRow(ctx,
		`INSERT INTO objects (collection_id, content_hash, date, size_bytes, data_type, expires_at, payload_key)
		 VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id, created_at`,
		collectionID, hash, date, sizeBytes, dataType, expiresAt, payloadKey,
	).Scan(&id, &createdAt)
	if err != nil {
		return nil, fmt.Errorf("insert object: %w", err)
	}
	return &models.Object{
		ID:          id,
		Collection:  "", // filled by caller if needed
		DataType:    dataType,
		Date:        date,
		SizeBytes:   sizeBytes,
		ContentHash: hash,
		CreatedAt:   createdAt,
		ExpiresAt:   expiresAt,
		PayloadKey:  payloadKey,
	}, nil
}

// DeleteObject removes object, tags, and returns payload_key.
func (d *DB) DeleteObject(ctx context.Context, id string) (string, error) {
	var payloadKey string
	err := d.pool.QueryRow(ctx,
		`DELETE FROM objects WHERE id = $1 RETURNING payload_key`,
		id,
	).Scan(&payloadKey)
	if err != nil {
		if err == pgx.ErrNoRows {
			return "", fmt.Errorf("object not found: %w", err)
		}
		return "", fmt.Errorf("delete object: %w", err)
	}
	return payloadKey, nil
}

// GetTagsForObject returns known tags for an object.
func (d *DB) GetTagsForObject(ctx context.Context, objectID string) (map[string]bool, error) {
	rows, err := d.pool.Query(ctx,
		`SELECT tag, value FROM object_tags WHERE object_id = $1`,
		objectID,
	)
	if err != nil {
		return nil, fmt.Errorf("get tags: %w", err)
	}
	defer rows.Close()

	tags := make(map[string]bool)
	for rows.Next() {
		var tag string
		var value bool
		if err := rows.Scan(&tag, &value); err != nil {
			return nil, fmt.Errorf("scan tag: %w", err)
		}
		tags[tag] = value
	}
	return tags, rows.Err()
}

// UpsertTags inserts or updates tags for an object.
func (d *DB) UpsertTags(ctx context.Context, collectionID, objectID string, tags map[string]bool) error {
	if len(tags) == 0 {
		return nil
	}
	tx, err := d.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)
	for tag, value := range tags {
		_, err := tx.Exec(ctx,
			`INSERT INTO object_tags (object_id, collection_id, tag, value, updated_at)
			 VALUES ($1, $2, $3, $4, NOW())
			 ON CONFLICT (object_id, tag) DO UPDATE SET value = EXCLUDED.value, updated_at = NOW()`,
			objectID, collectionID, tag, value,
		)
		if err != nil {
			return fmt.Errorf("upsert tag %s: %w", tag, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

// ListExpiredObjects returns expired object IDs in batches.
func (d *DB) ListExpiredObjects(ctx context.Context, limit int) ([]string, error) {
	rows, err := d.pool.Query(ctx,
		`SELECT id FROM objects WHERE expires_at IS NOT NULL AND expires_at <= NOW() ORDER BY expires_at LIMIT $1`,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list expired: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan expired id: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// QueryObjectsKnownTags returns objects that already have all tags known and matching.
func (d *DB) QueryObjectsKnownTags(ctx context.Context, collectionID string, tags map[string]bool, dateFilter *models.DateFilter, cursorDate time.Time, cursorID string, limit int) ([]models.Object, error) {
	// Build a query that finds objects where for every tag in the request,
	// there exists a row in object_tags with that value.
	// This is done via aggregation to avoid joins that could produce wrong counts.
	if len(tags) == 0 {
		return d.queryObjectsByDate(ctx, collectionID, dateFilter, cursorDate, cursorID, limit)
	}

	args := []any{collectionID}
	argIdx := 1

	var conds []string

	if cursorID != "" {
		argIdx++
		conds = append(conds, fmt.Sprintf("(o.date < $%d OR (o.date = $%d AND o.id > $%d))", argIdx, argIdx, argIdx+1))
		args = append(args, cursorDate, cursorID)
		argIdx += 2
	}

	// date filter
	dateConds, dateArgs, di := buildDateConds("o.date", argIdx, dateFilter)
	if len(dateConds) > 0 {
		conds = append(conds, dateConds...)
		args = append(args, dateArgs...)
		argIdx = di
	}

	// We need objects where all requested tags are known and match.
	// Strategy: use a lateral subquery or CTE. Let's use a CTE.
	// Actually, simpler: select from objects where id IN (
	//   SELECT object_id FROM object_tags WHERE collection_id=$1 AND (tag=$2 AND value=$3 OR ...)
	//   GROUP BY object_id HAVING COUNT(*) = N
	// ) and then order by date desc, id asc.

	tagConds := []string{}
	tagIdx := 0
	for tag, value := range tags {
		argIdx++
		cond := fmt.Sprintf("(tag = $%d AND value = $%d)", argIdx, argIdx+1)
		tagConds = append(tagConds, cond)
		args = append(args, tag, value)
		argIdx++
		tagIdx++
	}

	args = append(args, limit)
	limitArg := len(args)

	whereClause := ""
	if len(conds) > 0 {
		whereClause = "AND " + stringsJoin(" AND ", conds)
	}

	tagWhere := stringsJoin(" OR ", tagConds)

	query := fmt.Sprintf(`
		WITH matching_objects AS (
			SELECT object_id
			FROM object_tags
			WHERE collection_id = $1 AND (%s)
			GROUP BY object_id
			HAVING COUNT(*) = %d
		)
		SELECT o.id, o.collection_id, c.name, o.data_type, o.date, o.size_bytes, o.content_hash, o.created_at, o.expires_at, o.payload_key
		FROM objects o
		JOIN collections c ON o.collection_id = c.id
		WHERE o.collection_id = $1 %s
		  AND (o.expires_at IS NULL OR o.expires_at > NOW())
		  AND o.id IN (SELECT object_id FROM matching_objects)
		ORDER BY o.date DESC, o.id ASC
		LIMIT $%d
	`, tagWhere, len(tags), whereClause, limitArg)

	rows, err := d.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query known tags: %w", err)
	}
	defer rows.Close()

	return scanObjects(rows)
}

func (d *DB) queryObjectsByDate(ctx context.Context, collectionID string, dateFilter *models.DateFilter, cursorDate time.Time, cursorID string, limit int) ([]models.Object, error) {
	args := []any{collectionID}
	argIdx := 1

	var conds []string

	if cursorID != "" {
		argIdx++
		conds = append(conds, fmt.Sprintf("(date < $%d OR (date = $%d AND id > $%d))", argIdx, argIdx, argIdx+1))
		args = append(args, cursorDate, cursorID)
		argIdx += 2
	}

	dateConds, dateArgs, di := buildDateConds("date", argIdx, dateFilter)
	if len(dateConds) > 0 {
		conds = append(conds, dateConds...)
		args = append(args, dateArgs...)
		argIdx = di
	}

	args = append(args, limit)
	limitArg := len(args)

	whereClause := ""
	if len(conds) > 0 {
		whereClause = "AND " + stringsJoin(" AND ", conds)
	}

	query := fmt.Sprintf(`
		SELECT o.id, o.collection_id, c.name, o.data_type, o.date, o.size_bytes, o.content_hash, o.created_at, o.expires_at, o.payload_key
		FROM objects o
		JOIN collections c ON o.collection_id = c.id
		WHERE o.collection_id = $1 %s
		  AND (o.expires_at IS NULL OR o.expires_at > NOW())
		ORDER BY o.date DESC, o.id ASC
		LIMIT $%d
	`, whereClause, limitArg)

	rows, err := d.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query by date: %w", err)
	}
	defer rows.Close()

	return scanObjects(rows)
}

// ScanCandidateObjects returns objects in order for further tag evaluation.
func (d *DB) ScanCandidateObjects(ctx context.Context, collectionID string, dateFilter *models.DateFilter, cursorDate time.Time, cursorID string, limit int) ([]models.Object, error) {
	args := []any{collectionID}
	argIdx := 1

	var conds []string

	if cursorID != "" {
		argIdx++
		conds = append(conds, fmt.Sprintf("(o.date < $%d OR (o.date = $%d AND o.id > $%d))", argIdx, argIdx, argIdx+1))
		args = append(args, cursorDate, cursorID)
		argIdx += 2
	}

	dateConds, dateArgs, di := buildDateConds("o.date", argIdx, dateFilter)
	if len(dateConds) > 0 {
		conds = append(conds, dateConds...)
		args = append(args, dateArgs...)
		argIdx = di
	}

	args = append(args, limit)
	limitArg := len(args)

	whereClause := ""
	if len(conds) > 0 {
		whereClause = "AND " + stringsJoin(" AND ", conds)
	}

	query := fmt.Sprintf(`
		SELECT o.id, o.collection_id, c.name, o.data_type, o.date, o.size_bytes, o.content_hash, o.created_at, o.expires_at, o.payload_key
		FROM objects o
		JOIN collections c ON o.collection_id = c.id
		WHERE o.collection_id = $1 %s
		  AND (o.expires_at IS NULL OR o.expires_at > NOW())
		ORDER BY o.date DESC, o.id ASC
		LIMIT $%d
	`, whereClause, limitArg)

	rows, err := d.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("scan candidates: %w", err)
	}
	defer rows.Close()

	return scanObjects(rows)
}

// GetKnownTagsForObjects returns all known tags for a set of object IDs.
func (d *DB) GetKnownTagsForObjects(ctx context.Context, objectIDs []string) (map[string]map[string]bool, error) {
	if len(objectIDs) == 0 {
		return map[string]map[string]bool{}, nil
	}
	// pgx.In doesn't work with string slices directly in v5 the same way.
	// We'll construct IN clause manually.
	args := make([]any, len(objectIDs))
	placeholders := make([]string, len(objectIDs))
	for i, id := range objectIDs {
		args[i] = id
		placeholders[i] = fmt.Sprintf("$%d", i+1)
	}
	query := fmt.Sprintf(`SELECT object_id, tag, value FROM object_tags WHERE object_id IN (%s)`, joinStrings(placeholders, ","))
	rows, err := d.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("get known tags: %w", err)
	}
	defer rows.Close()

	result := make(map[string]map[string]bool)
	for rows.Next() {
		var oid, tag string
		var value bool
		if err := rows.Scan(&oid, &tag, &value); err != nil {
			return nil, fmt.Errorf("scan known tag: %w", err)
		}
		if result[oid] == nil {
			result[oid] = make(map[string]bool)
		}
		result[oid][tag] = value
	}
	return result, rows.Err()
}

func buildDateConds(col string, startIdx int, df *models.DateFilter) ([]string, []any, int) {
	if df == nil {
		return nil, nil, startIdx
	}
	var conds []string
	var args []any
	idx := startIdx
	if df.GT != nil {
		idx++
		conds = append(conds, fmt.Sprintf("%s > $%d", col, idx))
		args = append(args, *df.GT)
	}
	if df.GTE != nil {
		idx++
		conds = append(conds, fmt.Sprintf("%s >= $%d", col, idx))
		args = append(args, *df.GTE)
	}
	if df.LT != nil {
		idx++
		conds = append(conds, fmt.Sprintf("%s < $%d", col, idx))
		args = append(args, *df.LT)
	}
	if df.LTE != nil {
		idx++
		conds = append(conds, fmt.Sprintf("%s <= $%d", col, idx))
		args = append(args, *df.LTE)
	}
	if df.EQ != nil {
		idx++
		conds = append(conds, fmt.Sprintf("%s = $%d", col, idx))
		args = append(args, *df.EQ)
	}
	return conds, args, idx
}

func stringsJoin(sep string, items []string) string {
	if len(items) == 0 {
		return ""
	}
	result := items[0]
	for i := 1; i < len(items); i++ {
		result += sep + items[i]
	}
	return result
}

func joinStrings(items []string, sep string) string {
	return stringsJoin(sep, items)
}

func scanObjects(rows pgx.Rows) ([]models.Object, error) {
	defer rows.Close()
	var objs []models.Object
	for rows.Next() {
		var o models.Object
		var expiresAt *time.Time
		if err := rows.Scan(&o.ID, &o.CollectionID, &o.Collection, &o.DataType, &o.Date, &o.SizeBytes, &o.ContentHash, &o.CreatedAt, &expiresAt, &o.PayloadKey); err != nil {
			return nil, fmt.Errorf("scan object: %w", err)
		}
		o.ExpiresAt = expiresAt
		objs = append(objs, o)
	}
	return objs, rows.Err()
}
