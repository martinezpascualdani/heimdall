package storage

import (
	"database/sql"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/martinezpascualdani/heimdall/internal/target-service/domain"
)

// PostgresStore implements target-service persistence.
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
	if err := migrateTargetService(db); err != nil {
		return nil, err
	}
	return &PostgresStore{db: db}, nil
}

func migrateTargetService(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS targets (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			name TEXT NOT NULL,
			description TEXT DEFAULT '',
			active BOOLEAN NOT NULL DEFAULT true,
			materialization_policy JSONB,
			tags TEXT[],
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
		CREATE INDEX IF NOT EXISTS idx_targets_active ON targets(active);
		CREATE INDEX IF NOT EXISTS idx_targets_name ON targets(name);

		CREATE TABLE IF NOT EXISTS target_rules (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			target_id UUID NOT NULL REFERENCES targets(id) ON DELETE CASCADE,
			kind VARCHAR(16) NOT NULL,
			selector_type VARCHAR(32) NOT NULL,
			selector_value TEXT NOT NULL DEFAULT '',
			address_family VARCHAR(8) DEFAULT 'all',
			rule_order INT NOT NULL DEFAULT 0,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
		CREATE INDEX IF NOT EXISTS idx_target_rules_target_id ON target_rules(target_id);

		CREATE TABLE IF NOT EXISTS target_materializations (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			target_id UUID NOT NULL REFERENCES targets(id) ON DELETE RESTRICT,
			materialized_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			total_prefix_count INT NOT NULL DEFAULT 0,
			status VARCHAR(32) NOT NULL,
			error_message TEXT,
			status_detail TEXT,
			scope_snapshot_ref JSONB,
			routing_snapshot_ref JSONB
		);
		CREATE INDEX IF NOT EXISTS idx_target_materializations_target_id ON target_materializations(target_id);
		CREATE INDEX IF NOT EXISTS idx_target_materializations_materialized_at ON target_materializations(materialized_at DESC);

		CREATE TABLE IF NOT EXISTS target_entries (
			materialization_id UUID NOT NULL REFERENCES target_materializations(id) ON DELETE CASCADE,
			prefix TEXT NOT NULL,
			PRIMARY KEY (materialization_id, prefix)
		);
		CREATE INDEX IF NOT EXISTS idx_target_entries_materialization_id ON target_entries(materialization_id);
		CREATE INDEX IF NOT EXISTS idx_target_entries_prefix ON target_entries(prefix);
	`)
	return err
}

func (s *PostgresStore) Close() error {
	return s.db.Close()
}

func (s *PostgresStore) Ping() error {
	return s.db.Ping()
}

// CreateTarget inserts a target and returns it with ID.
func (s *PostgresStore) CreateTarget(t *domain.Target) error {
	t.CreatedAt = time.Now()
	t.UpdatedAt = t.CreatedAt
	if t.ID == uuid.Nil {
		t.ID = uuid.New()
	}
	policy := interface{}(t.MaterializationPolicy)
	if len(t.MaterializationPolicy) == 0 {
		policy = nil
	}
	_, err := s.db.Exec(`
		INSERT INTO targets (id, name, description, active, materialization_policy, tags, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, t.ID, t.Name, t.Description, t.Active, policy, pq.Array(t.Tags), t.CreatedAt, t.UpdatedAt)
	return err
}

// GetTargetByID returns a target by ID or nil if not found.
func (s *PostgresStore) GetTargetByID(id uuid.UUID) (*domain.Target, error) {
	var t domain.Target
	var policy []byte
	var tags pq.StringArray
	err := s.db.QueryRow(`
		SELECT id, name, description, active, materialization_policy, tags, created_at, updated_at
		FROM targets WHERE id = $1
	`, id).Scan(&t.ID, &t.Name, &t.Description, &t.Active, &policy, &tags, &t.CreatedAt, &t.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	t.MaterializationPolicy = policy
	t.Tags = tags
	return &t, nil
}

// ListTargets returns targets with pagination. activeOnly: when true, only active=true.
func (s *PostgresStore) ListTargets(limit, offset int, activeOnly bool) ([]*domain.Target, error) {
	q := `
		SELECT id, name, description, active, materialization_policy, tags, created_at, updated_at
		FROM targets
	`
	args := []interface{}{}
	if activeOnly {
		q += ` WHERE active = true `
	}
	q += ` ORDER BY name ASC LIMIT $1 OFFSET $2`
	args = append(args, limit, offset)
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*domain.Target
	for rows.Next() {
		var t domain.Target
		var policy []byte
		var tags pq.StringArray
		if err := rows.Scan(&t.ID, &t.Name, &t.Description, &t.Active, &policy, &tags, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		t.MaterializationPolicy = policy
		t.Tags = tags
		out = append(out, &t)
	}
	return out, rows.Err()
}

// UpdateTarget full replacement of target fields (name, description, active, policy, tags).
func (s *PostgresStore) UpdateTarget(t *domain.Target) error {
	t.UpdatedAt = time.Now()
	policy := interface{}(t.MaterializationPolicy)
	if len(t.MaterializationPolicy) == 0 {
		policy = nil
	}
	_, err := s.db.Exec(`
		UPDATE targets SET name = $1, description = $2, active = $3, materialization_policy = $4, tags = $5, updated_at = $6
		WHERE id = $7
	`, t.Name, t.Description, t.Active, policy, pq.Array(t.Tags), t.UpdatedAt, t.ID)
	return err
}

// SoftDeleteTarget sets active = false. Idempotent.
func (s *PostgresStore) SoftDeleteTarget(id uuid.UUID) error {
	_, err := s.db.Exec(`UPDATE targets SET active = false, updated_at = NOW() WHERE id = $1`, id)
	return err
}

// InsertRules replaces rules for a target: delete existing, insert new.
func (s *PostgresStore) InsertRules(targetID uuid.UUID, rules []domain.TargetRule) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.Exec(`DELETE FROM target_rules WHERE target_id = $1`, targetID)
	if err != nil {
		return err
	}
	for i := range rules {
		r := &rules[i]
		if r.ID == uuid.Nil {
			r.ID = uuid.New()
		}
		r.TargetID = targetID
		r.CreatedAt = time.Now()
		_, err = tx.Exec(`
			INSERT INTO target_rules (id, target_id, kind, selector_type, selector_value, address_family, rule_order, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		`, r.ID, r.TargetID, r.Kind, r.SelectorType, r.SelectorValue, r.AddressFamily, r.RuleOrder, r.CreatedAt)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ListRulesByTargetID returns all rules for a target, ordered by rule_order, kind.
func (s *PostgresStore) ListRulesByTargetID(targetID uuid.UUID) ([]domain.TargetRule, error) {
	rows, err := s.db.Query(`
		SELECT id, target_id, kind, selector_type, selector_value, address_family, rule_order, created_at
		FROM target_rules WHERE target_id = $1 ORDER BY rule_order ASC, kind ASC
	`, targetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.TargetRule
	for rows.Next() {
		var r domain.TargetRule
		if err := rows.Scan(&r.ID, &r.TargetID, &r.Kind, &r.SelectorType, &r.SelectorValue, &r.AddressFamily, &r.RuleOrder, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// jsonbArg returns nil for empty JSON so PostgreSQL accepts it as NULL.
func jsonbArg(b json.RawMessage) interface{} {
	if len(b) == 0 {
		return nil
	}
	return b
}

// CreateMaterialization inserts a new materialization row (status running).
func (s *PostgresStore) CreateMaterialization(m *domain.TargetMaterialization) error {
	if m.ID == uuid.Nil {
		m.ID = uuid.New()
	}
	if m.MaterializedAt.IsZero() {
		m.MaterializedAt = time.Now()
	}
	_, err := s.db.Exec(`
		INSERT INTO target_materializations (id, target_id, materialized_at, total_prefix_count, status, error_message, status_detail, scope_snapshot_ref, routing_snapshot_ref)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, m.ID, m.TargetID, m.MaterializedAt, m.TotalPrefixCount, m.Status, m.ErrorMessage, m.StatusDetail, jsonbArg(m.ScopeSnapshotRef), jsonbArg(m.RoutingSnapshotRef))
	return err
}

// UpdateMaterialization updates status, total_prefix_count, refs, error_message.
func (s *PostgresStore) UpdateMaterialization(m *domain.TargetMaterialization) error {
	_, err := s.db.Exec(`
		UPDATE target_materializations SET total_prefix_count = $1, status = $2, error_message = $3, status_detail = $4, scope_snapshot_ref = $5, routing_snapshot_ref = $6
		WHERE id = $7
	`, m.TotalPrefixCount, m.Status, m.ErrorMessage, m.StatusDetail, jsonbArg(m.ScopeSnapshotRef), jsonbArg(m.RoutingSnapshotRef), m.ID)
	return err
}

// GetMaterializationByID returns a materialization by ID or nil.
func (s *PostgresStore) GetMaterializationByID(id uuid.UUID) (*domain.TargetMaterialization, error) {
	var m domain.TargetMaterialization
	var scopeRef, routingRef []byte
	err := s.db.QueryRow(`
		SELECT id, target_id, materialized_at, total_prefix_count, status, error_message, status_detail, scope_snapshot_ref, routing_snapshot_ref
		FROM target_materializations WHERE id = $1
	`, id).Scan(&m.ID, &m.TargetID, &m.MaterializedAt, &m.TotalPrefixCount, &m.Status, &m.ErrorMessage, &m.StatusDetail, &scopeRef, &routingRef)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	m.ScopeSnapshotRef = scopeRef
	m.RoutingSnapshotRef = routingRef
	return &m, nil
}

// ListMaterializations returns materializations for a target, ordered by materialized_at DESC.
func (s *PostgresStore) ListMaterializations(targetID uuid.UUID, limit, offset int) ([]*domain.TargetMaterialization, error) {
	rows, err := s.db.Query(`
		SELECT id, target_id, materialized_at, total_prefix_count, status, error_message, status_detail, scope_snapshot_ref, routing_snapshot_ref
		FROM target_materializations WHERE target_id = $1 ORDER BY materialized_at DESC LIMIT $2 OFFSET $3
	`, targetID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*domain.TargetMaterialization
	for rows.Next() {
		var m domain.TargetMaterialization
		var scopeRef, routingRef []byte
		if err := rows.Scan(&m.ID, &m.TargetID, &m.MaterializedAt, &m.TotalPrefixCount, &m.Status, &m.ErrorMessage, &m.StatusDetail, &scopeRef, &routingRef); err != nil {
			return nil, err
		}
		m.ScopeSnapshotRef = scopeRef
		m.RoutingSnapshotRef = routingRef
		out = append(out, &m)
	}
	return out, rows.Err()
}

// InsertTargetEntries batch-inserts entries for a materialization.
func (s *PostgresStore) InsertTargetEntries(materializationID uuid.UUID, prefixes []string) error {
	if len(prefixes) == 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(`INSERT INTO target_entries (materialization_id, prefix) VALUES ($1, $2) ON CONFLICT (materialization_id, prefix) DO NOTHING`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, p := range prefixes {
		_, err = stmt.Exec(materializationID, p)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ListPrefixes returns paginated prefixes for a materialization.
func (s *PostgresStore) ListPrefixes(materializationID uuid.UUID, limit, offset int) ([]string, error) {
	rows, err := s.db.Query(`
		SELECT prefix FROM target_entries WHERE materialization_id = $1 ORDER BY prefix ASC LIMIT $2 OFFSET $3
	`, materializationID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// CountPrefixes returns total prefix count for a materialization.
func (s *PostgresStore) CountPrefixes(materializationID uuid.UUID) (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM target_entries WHERE materialization_id = $1`, materializationID).Scan(&n)
	return n, err
}

// GetAllPrefixesForMaterialization returns all prefixes (for diff). Use with care on large sets.
func (s *PostgresStore) GetAllPrefixesForMaterialization(materializationID uuid.UUID) ([]string, error) {
	rows, err := s.db.Query(`SELECT prefix FROM target_entries WHERE materialization_id = $1 ORDER BY prefix ASC`, materializationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// GetMaterializationTargetID returns target_id for a materialization (for diff validation).
func (s *PostgresStore) GetMaterializationTargetID(materializationID uuid.UUID) (uuid.UUID, error) {
	var targetID uuid.UUID
	err := s.db.QueryRow(`SELECT target_id FROM target_materializations WHERE id = $1`, materializationID).Scan(&targetID)
	if err == sql.ErrNoRows {
		return uuid.Nil, nil
	}
	return targetID, err
}

// DeleteEntriesForMaterialization removes all entries for a materialization (on failed run cleanup).
func (s *PostgresStore) DeleteEntriesForMaterialization(materializationID uuid.UUID) error {
	_, err := s.db.Exec(`DELETE FROM target_entries WHERE materialization_id = $1`, materializationID)
	return err
}

// Helper to marshal snapshot ref for storage.
func MarshalSnapshotRef(ref *domain.SnapshotRef) (json.RawMessage, error) {
	if ref == nil {
		return nil, nil
	}
	return json.Marshal(ref)
}
