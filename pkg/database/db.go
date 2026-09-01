package database

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository manages PostgreSQL operations with Row-Level Security (RLS).
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository creates a database repository initialized with a pgx connection pool.
func NewRepository(ctx context.Context, connString string) (*Repository, error) {
	config, err := pgxpool.ParseConfig(connString)
	if err != nil {
		return nil, fmt.Errorf("failed to parse postgres connection string: %w", err)
	}

	config.MaxConns = 50
	config.MinConns = 5
	config.MaxConnIdleTime = 15 * time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("failed to create postgres connection pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to ping postgres database: %w", err)
	}

	return &Repository{pool: pool}, nil
}

// Close closes the underlying database pool.
func (r *Repository) Close() {
	if r.pool != nil {
		r.pool.Close()
	}
}

// WithTenantTx executes a database transaction with RLS enforced via SET LOCAL app.current_tenant_id = $1.
func (r *Repository) WithTenantTx(ctx context.Context, tenantID string, fn func(tx pgx.Tx) error) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin postgres transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// LLD Section 5.3: Set local tenant context for Row-Level Security isolation
	setRLSSQL := "SET LOCAL app.current_tenant_id = $1"
	if _, err := tx.Exec(ctx, setRLSSQL, tenantID); err != nil {
		return fmt.Errorf("failed to set RLS app.current_tenant_id for tenant '%s': %w", tenantID, err)
	}

	if err := fn(tx); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// UpsertTenant inserts a tenant record if it does not exist.
func (r *Repository) UpsertTenant(ctx context.Context, tenantID, name string) error {
	query := `
	INSERT INTO tenants (tenant_id, name)
	VALUES ($1, $2)
	ON CONFLICT (tenant_id) DO UPDATE SET name = EXCLUDED.name;`

	_, err := r.pool.Exec(ctx, query, tenantID, name)
	return err
}

// SaveDevice persists endpoint telemetry inside an RLS tenant isolated transaction.
func (r *Repository) SaveDevice(ctx context.Context, tenantID, deviceID, hostname string, hardwareSpecsJSON []byte) error {
	return r.WithTenantTx(ctx, tenantID, func(tx pgx.Tx) error {
		query := `
		INSERT INTO devices (device_id, tenant_id, hostname, hardware_specs, last_sync)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (device_id) DO UPDATE SET
			hostname = EXCLUDED.hostname,
			hardware_specs = EXCLUDED.hardware_specs,
			last_sync = EXCLUDED.last_sync;`

		_, err := tx.Exec(ctx, query, deviceID, tenantID, hostname, hardwareSpecsJSON, time.Now().UTC())
		if err != nil {
			return fmt.Errorf("failed to upsert device %s: %w", deviceID, err)
		}

		return nil
	})
}

// DeviceRecord represents a row from the devices table.
type DeviceRecord struct {
	DeviceID      string    `json:"device_id"`
	TenantID      string    `json:"tenant_id"`
	Hostname      string    `json:"hostname"`
	HardwareSpecs []byte    `json:"hardware_specs"`
	LastSync      time.Time `json:"last_sync"`
}

// GetDevicesForTenant retrieves devices for a tenant under RLS protection.
func (r *Repository) GetDevicesForTenant(ctx context.Context, tenantID string) ([]DeviceRecord, error) {
	var results []DeviceRecord

	err := r.WithTenantTx(ctx, tenantID, func(tx pgx.Tx) error {
		query := `SELECT device_id, tenant_id, hostname, hardware_specs, last_sync FROM devices WHERE tenant_id = $1`
		rows, err := tx.Query(ctx, query, tenantID)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var d DeviceRecord
			if err := rows.Scan(&d.DeviceID, &d.TenantID, &d.Hostname, &d.HardwareSpecs, &d.LastSync); err != nil {
				return err
			}
			results = append(results, d)
		}
		return nil
	})

	return results, err
}

// SaveOAuthToken persists encrypted OAuth token envelope in PostgreSQL BYTEA columns under RLS.
func (r *Repository) SaveOAuthToken(ctx context.Context, tokenID, tenantID, provider string, encryptedDEK, ciphertext []byte) error {
	return r.WithTenantTx(ctx, tenantID, func(tx pgx.Tx) error {
		query := `
		INSERT INTO oauth_tokens (token_id, tenant_id, provider, encrypted_dek, token_ciphertext)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (token_id) DO UPDATE SET
			encrypted_dek = EXCLUDED.encrypted_dek,
			token_ciphertext = EXCLUDED.token_ciphertext;`

		_, err := tx.Exec(ctx, query, tokenID, tenantID, provider, encryptedDEK, ciphertext)
		return err
	})
}

// SaveCloudCredential persists AWS RoleArn and ExternalId for a tenant under RLS.
func (r *Repository) SaveCloudCredential(ctx context.Context, credID, tenantID, roleArn, externalID string) error {
	return r.WithTenantTx(ctx, tenantID, func(tx pgx.Tx) error {
		query := `
		INSERT INTO cloud_credentials (credential_id, tenant_id, role_arn, external_id)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (credential_id) DO UPDATE SET
			role_arn = EXCLUDED.role_arn,
			external_id = EXCLUDED.external_id;`

		_, err := tx.Exec(ctx, query, credID, tenantID, roleArn, externalID)
		return err
	})
}

// GetCloudCredential retrieves AWS RoleArn and ExternalId under RLS protection.
func (r *Repository) GetCloudCredential(ctx context.Context, tenantID string) (roleArn string, externalID string, err error) {
	err = r.WithTenantTx(ctx, tenantID, func(tx pgx.Tx) error {
		query := `SELECT role_arn, external_id FROM cloud_credentials WHERE tenant_id = $1 LIMIT 1`
		return tx.QueryRow(ctx, query, tenantID).Scan(&roleArn, &externalID)
	})
	return roleArn, externalID, err
}
