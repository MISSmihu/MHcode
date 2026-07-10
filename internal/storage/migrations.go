package storage

type Migration struct {
	Version int
	Name    string
	SQL     string
}

func InitialMigrations() []Migration {
	return []Migration{
		{
			Version: 1,
			Name:    "usage_metrics",
			SQL:     "create table if not exists usage_metrics (id integer primary key, prompt_cache_hit_tokens integer, prompt_cache_miss_tokens integer, input_tokens integer, output_tokens integer, effective_cost real);",
		},
	}
}
