package storage

type Migration struct {
	Version    int
	Name       string
	Statements []string
}

func InitialMigrations() []Migration {
	return []Migration{
		{
			Version: 1,
			Name:    "usage_metrics",
			Statements: []string{
				`CREATE TABLE IF NOT EXISTS usage_metrics (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					prompt_cache_hit_tokens INTEGER NOT NULL DEFAULT 0,
					prompt_cache_miss_tokens INTEGER NOT NULL DEFAULT 0,
					input_tokens INTEGER NOT NULL DEFAULT 0,
					output_tokens INTEGER NOT NULL DEFAULT 0,
					effective_cost REAL NOT NULL DEFAULT 0
				)`,
			},
		},
		{
			Version: 2,
			Name:    "usage_dimensions",
			Statements: []string{
				"ALTER TABLE usage_metrics ADD COLUMN created_at TEXT NOT NULL DEFAULT ''",
				"ALTER TABLE usage_metrics ADD COLUMN session_id TEXT NOT NULL DEFAULT ''",
				"ALTER TABLE usage_metrics ADD COLUMN provider_id TEXT NOT NULL DEFAULT ''",
				"ALTER TABLE usage_metrics ADD COLUMN provider_name TEXT NOT NULL DEFAULT ''",
				"ALTER TABLE usage_metrics ADD COLUMN protocol TEXT NOT NULL DEFAULT ''",
				"ALTER TABLE usage_metrics ADD COLUMN model_id TEXT NOT NULL DEFAULT ''",
				"ALTER TABLE usage_metrics ADD COLUMN reasoning TEXT NOT NULL DEFAULT ''",
				"CREATE INDEX IF NOT EXISTS usage_metrics_session_id_id ON usage_metrics(session_id, id)",
				"CREATE INDEX IF NOT EXISTS usage_metrics_provider_model_id ON usage_metrics(provider_id, model_id, id)",
			},
		},
	}
}
