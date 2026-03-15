package storage

import (
	"database/sql"
	"net"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/martinezpascualdani/heimdall/internal/scope-service/domain"
	"github.com/lib/pq"
)

// PostgresStore implements scope-service persistence.
type PostgresStore struct {
	db *sql.DB
}

// NewPostgresStore opens a connection and runs migrations.
func NewPostgresStore(dsn string) (*PostgresStore, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		return nil, err
	}
	if err := migrateScopeService(db); err != nil {
		return nil, err
	}
	return &PostgresStore{db: db}, nil
}

func migrateScopeService(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS scope_imports (
			id UUID PRIMARY KEY,
			dataset_id UUID NOT NULL,
			config_effective TEXT NOT NULL,
			state VARCHAR(32) NOT NULL,
			blocks_persisted BIGINT DEFAULT 0,
			duration_ms BIGINT DEFAULT 0,
			error_text TEXT,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
		CREATE INDEX IF NOT EXISTS idx_scope_imports_dataset ON scope_imports(dataset_id);
		CREATE UNIQUE INDEX IF NOT EXISTS idx_scope_imports_idempotent ON scope_imports(dataset_id, config_effective) WHERE state = 'imported';

		CREATE TABLE IF NOT EXISTS scope_blocks (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			dataset_id UUID NOT NULL,
			scope_type VARCHAR(32) NOT NULL,
			scope_value VARCHAR(16) NOT NULL,
			address_family VARCHAR(8) NOT NULL,
			block_raw_identity TEXT NOT NULL,
			start_value VARCHAR(64),
			value_field VARCHAR(64),
			normalized_cidrs TEXT[],
			status VARCHAR(32),
			cc VARCHAR(4),
			date_value VARCHAR(16),
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			UNIQUE(dataset_id, scope_type, scope_value, block_raw_identity)
		);
		CREATE INDEX IF NOT EXISTS idx_scope_blocks_dataset_scope ON scope_blocks(dataset_id, scope_type, scope_value);
		CREATE INDEX IF NOT EXISTS idx_scope_blocks_scope ON scope_blocks(scope_type, scope_value);
	`);
	if err != nil {
		return err
	}
	if _, err := db.Exec(`ALTER TABLE scope_imports ADD COLUMN IF NOT EXISTS registry VARCHAR(32) DEFAULT ''`); err != nil {
		return err
	}
	if _, err := db.Exec(`ALTER TABLE scope_imports ADD COLUMN IF NOT EXISTS asns_persisted BIGINT DEFAULT 0`); err != nil {
		return err
	}
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS scope_asns (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			dataset_id UUID NOT NULL,
			scope_type VARCHAR(32) NOT NULL,
			scope_value VARCHAR(16) NOT NULL,
			asn_start BIGINT NOT NULL,
			asn_count BIGINT NOT NULL,
			status VARCHAR(32),
			cc VARCHAR(4),
			date_value VARCHAR(16),
			registry VARCHAR(32),
			raw_identity TEXT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			UNIQUE(dataset_id, scope_type, scope_value, raw_identity)
		);
		CREATE INDEX IF NOT EXISTS idx_scope_asns_dataset_scope ON scope_asns(dataset_id, scope_type, scope_value);
		CREATE INDEX IF NOT EXISTS idx_scope_asns_scope ON scope_asns(scope_type, scope_value);
		CREATE INDEX IF NOT EXISTS idx_scope_asns_scope_start ON scope_asns(scope_type, scope_value, asn_start);
	`)
	return err
}

// CreateImport inserts a ScopeImport.
func (s *PostgresStore) CreateImport(imp *domain.ScopeImport) error {
	_, err := s.db.Exec(`
		INSERT INTO scope_imports (id, dataset_id, registry, config_effective, state, blocks_persisted, asns_persisted, duration_ms, error_text, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`, imp.ID, imp.DatasetID, imp.Registry, imp.ConfigEffective, imp.State, imp.BlocksPersisted, imp.AsnsPersisted, imp.DurationMs, imp.Error, imp.CreatedAt)
	return err
}

// UpdateImportState updates state, blocks_persisted, asns_persisted and optional error.
func (s *PostgresStore) UpdateImportState(id uuid.UUID, state domain.ScopeImportState, blocksPersisted, asnsPersisted int64, errText string) error {
	_, err := s.db.Exec(`
		UPDATE scope_imports SET state = $1, blocks_persisted = $2, asns_persisted = $3, error_text = $4 WHERE id = $5
	`, state, blocksPersisted, asnsPersisted, errText, id)
	return err
}

// IPBlockMatch is the minimal data returned when an IP matches a scope block.
type IPBlockMatch struct {
	ScopeType  string
	ScopeValue string
	DatasetID  uuid.UUID
}

// FindBlockByIP returns the first scope block that contains the given IP for the given dataset (deterministic ORDER BY).
func (s *PostgresStore) FindBlockByIP(ip net.IP, datasetID uuid.UUID) (*IPBlockMatch, error) {
	ipStr := ip.String()
	if ip.To4() != nil {
		var m IPBlockMatch
		err := s.db.QueryRow(`
			SELECT scope_type, scope_value, dataset_id
			FROM scope_blocks
			WHERE dataset_id = $1 AND address_family = 'ipv4'
			  AND start_value IS NOT NULL AND value_field IS NOT NULL
			  AND $2::inet >= start_value::inet
			  AND $2::inet <= start_value::inet + (value_field::int - 1)
			ORDER BY scope_type, scope_value, block_raw_identity
			LIMIT 1
		`, datasetID, ipStr).Scan(&m.ScopeType, &m.ScopeValue, &m.DatasetID)
		if err == sql.ErrNoRows {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		return &m, nil
	}
	var m IPBlockMatch
	err := s.db.QueryRow(`
		SELECT scope_type, scope_value, dataset_id
		FROM scope_blocks
		WHERE dataset_id = $1 AND address_family = 'ipv6'
		  AND start_value IS NOT NULL AND value_field IS NOT NULL
		  AND $2::inet << (start_value || '/' || value_field)::cidr
		ORDER BY scope_type, scope_value, block_raw_identity
		LIMIT 1
	`, datasetID, ipStr).Scan(&m.ScopeType, &m.ScopeValue, &m.DatasetID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// FindBlockByIPInLatestPerRegistry finds the first block containing the IP in the latest imported dataset per registry only (one RIPE, one ARIN, one APNIC, etc.).
// Avoids mixing old and new versions of the same RIR; builds a consistent global snapshot.
func (s *PostgresStore) FindBlockByIPInLatestPerRegistry(ip net.IP) (*IPBlockMatch, error) {
	ipStr := ip.String()
	cte := `WITH latest_per_registry AS (
		SELECT DISTINCT ON (registry) dataset_id
		FROM scope_imports
		WHERE state = $1 AND registry IS NOT NULL AND registry != ''
		ORDER BY registry, created_at DESC
	)`
	if ip.To4() != nil {
		var m IPBlockMatch
		err := s.db.QueryRow(cte+`
			SELECT sb.scope_type, sb.scope_value, sb.dataset_id
			FROM scope_blocks sb
			WHERE sb.dataset_id IN (SELECT dataset_id FROM latest_per_registry)
			  AND sb.address_family = 'ipv4'
			  AND sb.start_value IS NOT NULL AND sb.value_field IS NOT NULL
			  AND $2::inet >= sb.start_value::inet
			  AND $2::inet <= sb.start_value::inet + (sb.value_field::int - 1)
			ORDER BY sb.scope_type, sb.scope_value, sb.block_raw_identity
			LIMIT 1
		`, domain.ImportStateImported, ipStr).Scan(&m.ScopeType, &m.ScopeValue, &m.DatasetID)
		if err == sql.ErrNoRows {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		return &m, nil
	}
	var m IPBlockMatch
	err := s.db.QueryRow(cte+`
		SELECT sb.scope_type, sb.scope_value, sb.dataset_id
		FROM scope_blocks sb
		WHERE sb.dataset_id IN (SELECT dataset_id FROM latest_per_registry)
		  AND sb.address_family = 'ipv6'
		  AND sb.start_value IS NOT NULL AND sb.value_field IS NOT NULL
		  AND $2::inet << (sb.start_value || '/' || sb.value_field)::cidr
		ORDER BY sb.scope_type, sb.scope_value, sb.block_raw_identity
		LIMIT 1
	`, domain.ImportStateImported, ipStr).Scan(&m.ScopeType, &m.ScopeValue, &m.DatasetID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// GetLatestImportedDatasetIDsPerRegistry returns one dataset_id per registry (latest imported), ordered by registry name.
// Used so country endpoints use the same global snapshot as IP resolution.
func (s *PostgresStore) GetLatestImportedDatasetIDsPerRegistry() ([]uuid.UUID, error) {
	rows, err := s.db.Query(`
		SELECT dataset_id FROM (
			SELECT DISTINCT ON (registry) dataset_id, registry
			FROM scope_imports
			WHERE state = $1 AND registry IS NOT NULL AND registry != ''
			ORDER BY registry, created_at DESC
		) t ORDER BY registry
	`, domain.ImportStateImported)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// GetLatestImportedDatasetIDForScope returns the dataset_id of the most recent import that has at least one block for the given scope.
// Used by country/blocks and country/summary when no dataset_id is provided: one coherent snapshot for that scope (e.g. ES → latest RIPE).
func (s *PostgresStore) GetLatestImportedDatasetIDForScope(scopeType, scopeValue string) (*uuid.UUID, error) {
	var id uuid.UUID
	err := s.db.QueryRow(`
		SELECT si.dataset_id FROM scope_imports si
		INNER JOIN scope_blocks sb ON sb.dataset_id = si.dataset_id AND sb.scope_type = $1 AND sb.scope_value = $2
		WHERE si.state = $3
		ORDER BY si.created_at DESC
		LIMIT 1
	`, scopeType, scopeValue, domain.ImportStateImported).Scan(&id)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &id, nil
}

// GetRegistriesByDatasetIDs returns dataset_id -> registry for each imported dataset (from scope_imports). Used to fill datasets_used without calling dataset-service.
func (s *PostgresStore) GetRegistriesByDatasetIDs(datasetIDs []uuid.UUID) (map[uuid.UUID]string, error) {
	if len(datasetIDs) == 0 {
		return nil, nil
	}
	rows, err := s.db.Query(`
		SELECT DISTINCT ON (dataset_id) dataset_id, COALESCE(registry, '')
		FROM scope_imports
		WHERE dataset_id = ANY($1) AND state = $2
		ORDER BY dataset_id, created_at DESC
	`, pq.Array(datasetIDs), domain.ImportStateImported)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[uuid.UUID]string)
	for rows.Next() {
		var id uuid.UUID
		var reg string
		if err := rows.Scan(&id, &reg); err != nil {
			return nil, err
		}
		out[id] = reg
	}
	return out, rows.Err()
}

// HasImportedDataset returns true only if there is a row in scope_imports with the given dataset_id and state = 'imported'.
func (s *PostgresStore) HasImportedDataset(datasetID uuid.UUID) (bool, error) {
	var dummy int
	err := s.db.QueryRow(`SELECT 1 FROM scope_imports WHERE dataset_id = $1 AND state = $2 LIMIT 1`, datasetID, domain.ImportStateImported).Scan(&dummy)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// FindImportByDatasetAndConfig returns an import with state=imported for the same dataset_id and config_effective.
func (s *PostgresStore) FindImportByDatasetAndConfig(datasetID uuid.UUID, configEffective string) (*domain.ScopeImport, error) {
	var imp domain.ScopeImport
	err := s.db.QueryRow(`
		SELECT id, dataset_id, COALESCE(registry, ''), config_effective, state, blocks_persisted, COALESCE(asns_persisted, 0), duration_ms, error_text, created_at
		FROM scope_imports WHERE dataset_id = $1 AND config_effective = $2 AND state = $3 LIMIT 1
	`, datasetID, configEffective, domain.ImportStateImported).Scan(&imp.ID, &imp.DatasetID, &imp.Registry, &imp.ConfigEffective, &imp.State, &imp.BlocksPersisted, &imp.AsnsPersisted, &imp.DurationMs, &imp.Error, &imp.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &imp, nil
}

// UpsertBlock inserts or ignores a block (unique on dataset_id, scope_type, scope_value, block_raw_identity).
func (s *PostgresStore) UpsertBlock(b *domain.ScopeBlock) error {
	_, err := s.db.Exec(`
		INSERT INTO scope_blocks (dataset_id, scope_type, scope_value, address_family, block_raw_identity, start_value, value_field, normalized_cidrs, status, cc, date_value, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		ON CONFLICT (dataset_id, scope_type, scope_value, block_raw_identity) DO NOTHING
	`, b.DatasetID, b.ScopeType, b.ScopeValue, b.AddressFamily, b.BlockRawIdentity, b.Start, b.Value, pq.Array(b.NormalizedCIDRs), b.Status, b.CC, b.Date, b.CreatedAt)
	return err
}


// ListBlocksByScope returns blocks for scope_type and scope_value, filtered by datasetIDs (e.g. latest per registry) and optional address_family.
// Order is address_family, start_value as IP (numeric order), block_raw_identity for human-friendly and deterministic output.
func (s *PostgresStore) ListBlocksByScope(scopeType, scopeValue string, datasetIDs []uuid.UUID, addressFamily string, limit, offset int) ([]*domain.ScopeBlock, error) {
	if len(datasetIDs) == 0 {
		return nil, nil
	}
	query := `
		SELECT id, dataset_id, scope_type, scope_value, address_family, block_raw_identity, start_value, value_field, normalized_cidrs, status, cc, date_value, created_at
		FROM scope_blocks
		WHERE scope_type = $1 AND scope_value = $2 AND dataset_id = ANY($3)
	`
	args := []interface{}{scopeType, scopeValue, pq.Array(datasetIDs)}
	n := 4
	if addressFamily == "ipv4" || addressFamily == "ipv6" {
		query += ` AND address_family = $` + strconv.Itoa(n) + `
		`
		args = append(args, addressFamily)
		n++
	}
	// Order by IP value (inet) so 2.x.x.x comes before 10.x, not after 103.x lexicographically
	query += ` ORDER BY address_family, (start_value::inet) NULLS LAST, block_raw_identity LIMIT $` + strconv.Itoa(n) + ` OFFSET $` + strconv.Itoa(n+1)
	args = append(args, limit, offset)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*domain.ScopeBlock
	for rows.Next() {
		var b domain.ScopeBlock
		var cidrs pq.StringArray
		err := rows.Scan(&b.ID, &b.DatasetID, &b.ScopeType, &b.ScopeValue, &b.AddressFamily, &b.BlockRawIdentity, &b.Start, &b.Value, &cidrs, &b.Status, &b.CC, &b.Date, &b.CreatedAt)
		if err != nil {
			return nil, err
		}
		b.NormalizedCIDRs = []string(cidrs)
		out = append(out, &b)
	}
	return out, rows.Err()
}

// CountBlocksByScope returns total count for scope (for pagination). datasetIDs is the set of snapshots to include (e.g. latest per registry).
func (s *PostgresStore) CountBlocksByScope(scopeType, scopeValue string, datasetIDs []uuid.UUID, addressFamily string) (int64, error) {
	if len(datasetIDs) == 0 {
		return 0, nil
	}
	query := `SELECT COUNT(*) FROM scope_blocks WHERE scope_type = $1 AND scope_value = $2 AND dataset_id = ANY($3)`
	args := []interface{}{scopeType, scopeValue, pq.Array(datasetIDs)}
	n := 4
	if addressFamily == "ipv4" || addressFamily == "ipv6" {
		query += ` AND address_family = $` + strconv.Itoa(n)
		args = append(args, addressFamily)
	}
	var count int64
	err := s.db.QueryRow(query, args...).Scan(&count)
	return count, err
}

// ImportSummary holds dataset_id and created_at for an imported snapshot (scope_imports).
type ImportSummary struct {
	DatasetID uuid.UUID
	CreatedAt time.Time
}

// ListImportedDatasetsForScope returns imported dataset IDs that have blocks for the given scope, ordered by scope_imports.created_at DESC.
func (s *PostgresStore) ListImportedDatasetsForScope(scopeType, scopeValue string) ([]ImportSummary, error) {
	rows, err := s.db.Query(`
		SELECT DISTINCT si.dataset_id, si.created_at
		FROM scope_imports si
		JOIN scope_blocks sb ON sb.dataset_id = si.dataset_id AND sb.scope_type = $1 AND sb.scope_value = $2
		WHERE si.state = $3
		ORDER BY si.created_at DESC
	`, scopeType, scopeValue, domain.ImportStateImported)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ImportSummary
	for rows.Next() {
		var sum ImportSummary
		if err := rows.Scan(&sum.DatasetID, &sum.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, sum)
	}
	return out, rows.Err()
}

// UpsertASN inserts or ignores an ASN range (unique on dataset_id, scope_type, scope_value, raw_identity).
func (s *PostgresStore) UpsertASN(a *domain.ScopeASN) error {
	_, err := s.db.Exec(`
		INSERT INTO scope_asns (dataset_id, scope_type, scope_value, asn_start, asn_count, status, cc, date_value, registry, raw_identity, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (dataset_id, scope_type, scope_value, raw_identity) DO NOTHING
	`, a.DatasetID, a.ScopeType, a.ScopeValue, a.ASNStart, a.ASNCount, a.Status, a.CC, a.Date, a.Registry, a.RawIdentity, a.CreatedAt)
	return err
}

// ListASNsByScope returns ASN ranges for scope_type and scope_value, filtered by datasetIDs.
// Order: asn_start ASC, raw_identity for stable ordering when two ranges share the same asn_start.
func (s *PostgresStore) ListASNsByScope(scopeType, scopeValue string, datasetIDs []uuid.UUID, limit, offset int) ([]*domain.ScopeASN, error) {
	if len(datasetIDs) == 0 {
		return nil, nil
	}
	rows, err := s.db.Query(`
		SELECT id, dataset_id, scope_type, scope_value, asn_start, asn_count, status, cc, date_value, registry, raw_identity, created_at
		FROM scope_asns
		WHERE scope_type = $1 AND scope_value = $2 AND dataset_id = ANY($3)
		ORDER BY asn_start ASC, raw_identity
		LIMIT $4 OFFSET $5
	`, scopeType, scopeValue, pq.Array(datasetIDs), limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*domain.ScopeASN
	for rows.Next() {
		var a domain.ScopeASN
		err := rows.Scan(&a.ID, &a.DatasetID, &a.ScopeType, &a.ScopeValue, &a.ASNStart, &a.ASNCount, &a.Status, &a.CC, &a.Date, &a.Registry, &a.RawIdentity, &a.CreatedAt)
		if err != nil {
			return nil, err
		}
		out = append(out, &a)
	}
	return out, rows.Err()
}

// CountASNRangeByScope returns the number of ASN range rows for the scope (for pagination total).
func (s *PostgresStore) CountASNRangeByScope(scopeType, scopeValue string, datasetIDs []uuid.UUID) (int64, error) {
	if len(datasetIDs) == 0 {
		return 0, nil
	}
	var count int64
	err := s.db.QueryRow(`
		SELECT COUNT(*) FROM scope_asns WHERE scope_type = $1 AND scope_value = $2 AND dataset_id = ANY($3)
	`, scopeType, scopeValue, pq.Array(datasetIDs)).Scan(&count)
	return count, err
}

// SumASNCountByScope returns the sum of asn_count for the scope (total individual ASNs across ranges).
func (s *PostgresStore) SumASNCountByScope(scopeType, scopeValue string, datasetIDs []uuid.UUID) (int64, error) {
	if len(datasetIDs) == 0 {
		return 0, nil
	}
	var sum int64
	err := s.db.QueryRow(`
		SELECT COALESCE(SUM(asn_count), 0) FROM scope_asns WHERE scope_type = $1 AND scope_value = $2 AND dataset_id = ANY($3)
	`, scopeType, scopeValue, pq.Array(datasetIDs)).Scan(&sum)
	return sum, err
}

// Ping for readiness.
func (s *PostgresStore) Ping() error {
	return s.db.Ping()
}

// Close closes the DB.
func (s *PostgresStore) Close() error {
	return s.db.Close()
}
