package storage

type DB struct {
	path string
}

func Open(path string) (*DB, error) {
	return &DB{path: path}, nil
}

func (db *DB) Path() string {
	return db.path
}
