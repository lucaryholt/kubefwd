package main

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
	_ "modernc.org/sqlite"
)

const currentSchemaVersion = 1

// ConfigStore loads and persists configuration (YAML file or SQLite).
type ConfigStore interface {
	Load() (*Config, error)
	Save(*Config) error
	Description() string
}

// --- File-backed YAML ---

// FileConfigStore reads and writes a YAML file on disk.
type FileConfigStore struct {
	Path string
}

func (f *FileConfigStore) Description() string { return f.Path }

func (f *FileConfigStore) Load() (*Config, error) {
	return LoadConfig(f.Path)
}

func cloneConfig(cfg *Config) *Config {
	if cfg == nil {
		return nil
	}
	c := *cfg
	c.Services = append([]Service(nil), cfg.Services...)
	c.ProxyServices = append([]ProxyService(nil), cfg.ProxyServices...)
	c.AlternativeContexts = append([]AlternativeContext(nil), cfg.AlternativeContexts...)
	c.Presets = make([]Preset, len(cfg.Presets))
	for i := range cfg.Presets {
		c.Presets[i].Name = cfg.Presets[i].Name
		c.Presets[i].Services = append([]string(nil), cfg.Presets[i].Services...)
	}
	return &c
}

func (f *FileConfigStore) Save(cfg *Config) error {
	c := cloneConfig(cfg)
	ApplyConfigDefaults(c)
	if err := ValidateConfig(c); err != nil {
		return err
	}
	FinalizeConfig(c)
	out, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	dir := filepath.Dir(f.Path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".kubefwd-*.yaml")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(out); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return err
	}
	if err := os.Rename(tmpPath, f.Path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("replace config file: %w", err)
	}
	return nil
}

// --- SQLite ---

// SQLiteConfigStore persists config in normalized tables.
type SQLiteConfigStore struct {
	Path string
	db   *sql.DB
}

func openSQLite(path string) (*sql.DB, error) {
	dir := filepath.Dir(path)
	if dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		db.Close()
		return nil, err
	}
	if err := migrateSQLite(db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func migrateSQLite(db *sql.DB) error {
	var v sql.NullInt64
	_ = db.QueryRow(`PRAGMA user_version`).Scan(&v)
	if int(v.Int64) >= currentSchemaVersion {
		return nil
	}
	if int(v.Int64) == 0 {
		if err := createSchemaV1(db); err != nil {
			return err
		}
	}
	if _, err := db.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, currentSchemaVersion)); err != nil {
		return err
	}
	return nil
}

func createSchemaV1(db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS settings (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			cluster_context TEXT NOT NULL DEFAULT '',
			cluster_name TEXT NOT NULL DEFAULT '',
			namespace TEXT NOT NULL DEFAULT '',
			max_retries INTEGER NOT NULL DEFAULT -1,
			web_port INTEGER NOT NULL DEFAULT 8765,
			proxy_pod_name TEXT NOT NULL DEFAULT '',
			proxy_pod_image TEXT NOT NULL DEFAULT '',
			proxy_pod_context TEXT NOT NULL DEFAULT '',
			proxy_pod_namespace TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS alternative_contexts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			sort_order INTEGER NOT NULL,
			name TEXT NOT NULL,
			context TEXT NOT NULL,
			UNIQUE(name)
		)`,
		`CREATE TABLE IF NOT EXISTS presets (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			sort_order INTEGER NOT NULL,
			name TEXT NOT NULL UNIQUE
		)`,
		`CREATE TABLE IF NOT EXISTS preset_services (
			preset_id INTEGER NOT NULL REFERENCES presets(id) ON DELETE CASCADE,
			sort_order INTEGER NOT NULL,
			service_name TEXT NOT NULL,
			PRIMARY KEY (preset_id, sort_order)
		)`,
		`CREATE TABLE IF NOT EXISTS services (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL UNIQUE,
			service_name TEXT NOT NULL,
			remote_port INTEGER NOT NULL,
			local_port INTEGER NOT NULL,
			selected_by_default INTEGER NOT NULL,
			context TEXT NOT NULL DEFAULT '',
			namespace TEXT NOT NULL DEFAULT '',
			max_retries INTEGER,
			sql_tap_port INTEGER,
			sql_tap_driver TEXT NOT NULL DEFAULT '',
			sql_tap_grpc_port INTEGER,
			sql_tap_http_port INTEGER
		)`,
		`CREATE TABLE IF NOT EXISTS proxy_services (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL UNIQUE,
			target_host TEXT NOT NULL,
			target_port INTEGER NOT NULL,
			local_port INTEGER NOT NULL,
			selected_by_default INTEGER NOT NULL,
			proxy_pod_context TEXT NOT NULL DEFAULT '',
			proxy_pod_namespace TEXT NOT NULL DEFAULT '',
			max_retries INTEGER,
			sql_tap_port INTEGER,
			sql_tap_driver TEXT NOT NULL DEFAULT '',
			sql_tap_grpc_port INTEGER,
			sql_tap_http_port INTEGER
		)`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			return fmt.Errorf("schema: %w", err)
		}
	}
	return nil
}

// NewSQLiteConfigStore opens (and creates) a SQLite database at Path.
func NewSQLiteConfigStore(path string) (*SQLiteConfigStore, error) {
	db, err := openSQLite(path)
	if err != nil {
		return nil, err
	}
	return &SQLiteConfigStore{Path: path, db: db}, nil
}

func (s *SQLiteConfigStore) Description() string { return "sqlite:" + s.Path }

func (s *SQLiteConfigStore) Close() error {
	if s.db == nil {
		return nil
	}
	return s.db.Close()
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func intToBool(i int) bool { return i != 0 }

func sqlIntPtr(ns sql.NullInt64) *int {
	if !ns.Valid {
		return nil
	}
	v := int(ns.Int64)
	return &v
}

func optionalIntPtr(v *int) interface{} {
	if v == nil {
		return nil
	}
	return *v
}

// Load reads all tables and returns a validated Config.
func (s *SQLiteConfigStore) Load() (*Config, error) {
	var count int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM settings WHERE id = 1`).Scan(&count); err != nil {
		return nil, err
	}
	if count == 0 {
		return nil, ErrSQLiteEmpty
	}

	cfg := &Config{}

	row := s.db.QueryRow(`SELECT cluster_context, cluster_name, namespace, max_retries, web_port,
		proxy_pod_name, proxy_pod_image, proxy_pod_context, proxy_pod_namespace FROM settings WHERE id = 1`)
	if err := row.Scan(
		&cfg.ClusterContext, &cfg.ClusterName, &cfg.Namespace, &cfg.MaxRetries, &cfg.WebPort,
		&cfg.ProxyPodName, &cfg.ProxyPodImage, &cfg.ProxyPodContext, &cfg.ProxyPodNamespace,
	); err != nil {
		return nil, err
	}

	acRows, err := s.db.Query(`SELECT name, context FROM alternative_contexts ORDER BY sort_order, id`)
	if err != nil {
		return nil, err
	}
	for acRows.Next() {
		var ac AlternativeContext
		if err := acRows.Scan(&ac.Name, &ac.Context); err != nil {
			acRows.Close()
			return nil, err
		}
		cfg.AlternativeContexts = append(cfg.AlternativeContexts, ac)
	}
	acRows.Close()

	presetRows, err := s.db.Query(`SELECT id, name FROM presets ORDER BY sort_order, id`)
	if err != nil {
		return nil, err
	}
	type presetRow struct {
		id   int64
		name string
	}
	var presetList []presetRow
	for presetRows.Next() {
		var pr presetRow
		if err := presetRows.Scan(&pr.id, &pr.name); err != nil {
			presetRows.Close()
			return nil, err
		}
		presetList = append(presetList, pr)
	}
	presetRows.Close()

	for _, pr := range presetList {
		svRows, err := s.db.Query(`SELECT service_name FROM preset_services WHERE preset_id = ? ORDER BY sort_order`, pr.id)
		if err != nil {
			return nil, err
		}
		var names []string
		for svRows.Next() {
			var n string
			if err := svRows.Scan(&n); err != nil {
				svRows.Close()
				return nil, err
			}
			names = append(names, n)
		}
		svRows.Close()
		cfg.Presets = append(cfg.Presets, Preset{Name: pr.name, Services: names})
	}

	svcRows, err := s.db.Query(`SELECT name, service_name, remote_port, local_port, selected_by_default,
		context, namespace, max_retries, sql_tap_port, sql_tap_driver, sql_tap_grpc_port, sql_tap_http_port
		FROM services ORDER BY name`)
	if err != nil {
		return nil, err
	}
	for svcRows.Next() {
		var sv Service
		var maxR, stp, stg, sth sql.NullInt64
		var drv string
		var sel int
		if err := svcRows.Scan(&sv.Name, &sv.ServiceName, &sv.RemotePort, &sv.LocalPort, &sel,
			&sv.Context, &sv.Namespace, &maxR, &stp, &drv, &stg, &sth); err != nil {
			svcRows.Close()
			return nil, err
		}
		sv.SelectedByDefault = intToBool(sel)
		sv.MaxRetries = sqlIntPtr(maxR)
		sv.SqlTapPort = sqlIntPtr(stp)
		sv.SqlTapGrpcPort = sqlIntPtr(stg)
		sv.SqlTapHttpPort = sqlIntPtr(sth)
		if drv != "" {
			sv.SqlTapDriver = drv
		}
		cfg.Services = append(cfg.Services, sv)
	}
	svcRows.Close()

	pxRows, err := s.db.Query(`SELECT name, target_host, target_port, local_port, selected_by_default,
		proxy_pod_context, proxy_pod_namespace, max_retries, sql_tap_port, sql_tap_driver, sql_tap_grpc_port, sql_tap_http_port
		FROM proxy_services ORDER BY proxy_pod_context, proxy_pod_namespace, name`)
	if err != nil {
		return nil, err
	}
	for pxRows.Next() {
		var ps ProxyService
		var maxR, stp, stg, sth sql.NullInt64
		var drv string
		var sel int
		if err := pxRows.Scan(&ps.Name, &ps.TargetHost, &ps.TargetPort, &ps.LocalPort, &sel,
			&ps.ProxyPodContext, &ps.ProxyPodNamespace, &maxR, &stp, &drv, &stg, &sth); err != nil {
			pxRows.Close()
			return nil, err
		}
		ps.SelectedByDefault = intToBool(sel)
		ps.MaxRetries = sqlIntPtr(maxR)
		ps.SqlTapPort = sqlIntPtr(stp)
		ps.SqlTapGrpcPort = sqlIntPtr(stg)
		ps.SqlTapHttpPort = sqlIntPtr(sth)
		if drv != "" {
			ps.SqlTapDriver = drv
		}
		cfg.ProxyServices = append(cfg.ProxyServices, ps)
	}
	pxRows.Close()

	ApplyConfigDefaults(cfg)
	if err := ValidateConfig(cfg); err != nil {
		return nil, err
	}
	FinalizeConfig(cfg)
	return cfg, nil
}

// ErrSQLiteEmpty is returned when the SQLite store has no settings row yet.
var ErrSQLiteEmpty = errors.New("sqlite database has no configuration (use -import-yaml or import from the UI)")

// Save replaces all config tables with cfg.
func (s *SQLiteConfigStore) Save(cfg *Config) error {
	if cfg == nil {
		return fmt.Errorf("nil config")
	}
	c := cloneConfig(cfg)
	ApplyConfigDefaults(c)
	if err := ValidateConfig(c); err != nil {
		return err
	}
	// Do not require FinalizeConfig for save — store canonical values; Load will finalize.

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM preset_services`); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM presets`); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM alternative_contexts`); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM services`); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM proxy_services`); err != nil {
		return err
	}

	_, err = tx.Exec(`INSERT OR REPLACE INTO settings (id, cluster_context, cluster_name, namespace, max_retries, web_port,
		proxy_pod_name, proxy_pod_image, proxy_pod_context, proxy_pod_namespace) VALUES (1, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.ClusterContext, c.ClusterName, c.Namespace, c.MaxRetries, c.WebPort,
		c.ProxyPodName, c.ProxyPodImage, c.ProxyPodContext, c.ProxyPodNamespace)
	if err != nil {
		return err
	}

	for i, ac := range c.AlternativeContexts {
		_, err = tx.Exec(`INSERT INTO alternative_contexts (sort_order, name, context) VALUES (?, ?, ?)`,
			i, ac.Name, ac.Context)
		if err != nil {
			return err
		}
	}

	for i, pr := range c.Presets {
		res, err := tx.Exec(`INSERT INTO presets (sort_order, name) VALUES (?, ?)`, i, pr.Name)
		if err != nil {
			return err
		}
		pid, err := res.LastInsertId()
		if err != nil {
			return err
		}
		for j, sn := range pr.Services {
			_, err = tx.Exec(`INSERT INTO preset_services (preset_id, sort_order, service_name) VALUES (?, ?, ?)`,
				pid, j, sn)
			if err != nil {
				return err
			}
		}
	}

	for _, sv := range c.Services {
		_, err = tx.Exec(`INSERT INTO services (name, service_name, remote_port, local_port, selected_by_default,
			context, namespace, max_retries, sql_tap_port, sql_tap_driver, sql_tap_grpc_port, sql_tap_http_port)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			sv.Name, sv.ServiceName, sv.RemotePort, sv.LocalPort, boolToInt(sv.SelectedByDefault),
			sv.Context, sv.Namespace, optionalIntPtr(sv.MaxRetries), optionalIntPtr(sv.SqlTapPort),
			strings.TrimSpace(sv.SqlTapDriver), optionalIntPtr(sv.SqlTapGrpcPort), optionalIntPtr(sv.SqlTapHttpPort))
		if err != nil {
			return err
		}
	}

	for _, ps := range c.ProxyServices {
		_, err = tx.Exec(`INSERT INTO proxy_services (name, target_host, target_port, local_port, selected_by_default,
			proxy_pod_context, proxy_pod_namespace, max_retries, sql_tap_port, sql_tap_driver, sql_tap_grpc_port, sql_tap_http_port)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			ps.Name, ps.TargetHost, ps.TargetPort, ps.LocalPort, boolToInt(ps.SelectedByDefault),
			ps.ProxyPodContext, ps.ProxyPodNamespace, optionalIntPtr(ps.MaxRetries), optionalIntPtr(ps.SqlTapPort),
			strings.TrimSpace(ps.SqlTapDriver), optionalIntPtr(ps.SqlTapGrpcPort), optionalIntPtr(ps.SqlTapHttpPort))
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}
