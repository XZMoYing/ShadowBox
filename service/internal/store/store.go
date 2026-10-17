package store

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"
)

type Store struct{ DB *sql.DB }

func Open(dir string) (*Store, error) {
	if e := os.MkdirAll(dir, 0700); e != nil {
		return nil, e
	}
	if e := os.Chmod(dir, 0700); e != nil {
		return nil, e
	}
	path := filepath.Join(dir, "shadowbox.sqlite")
	f, e := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return nil, e
	}
	f.Close()
	_ = os.Chmod(path, 0600)
	db, e := sql.Open("sqlite3", path+"?_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=on")
	if e != nil {
		return nil, e
	}
	db.SetMaxOpenConns(1)
	_, e = db.Exec(`CREATE TABLE IF NOT EXISTS documents(collection TEXT NOT NULL,id TEXT NOT NULL,value BLOB NOT NULL,PRIMARY KEY(collection,id))`)
	if e != nil {
		db.Close()
		return nil, e
	}
	return &Store{db}, nil
}
func (s *Store) Put(collection, id string, v any) error {
	b, e := json.Marshal(v)
	if e != nil {
		return e
	}
	_, e = s.DB.Exec(`INSERT INTO documents VALUES(?,?,?) ON CONFLICT(collection,id) DO UPDATE SET value=excluded.value`, collection, id, b)
	return e
}
func (s *Store) Get(collection, id string, v any) error {
	var b []byte
	if e := s.DB.QueryRow(`SELECT value FROM documents WHERE collection=? AND id=?`, collection, id).Scan(&b); e != nil {
		return e
	}
	return json.Unmarshal(b, v)
}
func (s *Store) All(collection string, v any) error {
	rows, e := s.DB.Query(`SELECT value FROM documents WHERE collection=? ORDER BY rowid`, collection)
	if e != nil {
		return e
	}
	defer rows.Close()
	a := []json.RawMessage{}
	for rows.Next() {
		var b []byte
		if e = rows.Scan(&b); e != nil {
			return e
		}
		a = append(a, b)
	}
	if e = rows.Err(); e != nil {
		return e
	}
	b, e := json.Marshal(a)
	if e != nil {
		return e
	}
	return json.Unmarshal(b, v)
}
func (s *Store) Delete(collection, id string) error {
	_, e := s.DB.Exec(`DELETE FROM documents WHERE collection=? AND id=?`, collection, id)
	return e
}
func (s *Store) ReplaceNodes(source string, nodes any) error {
	b, e := json.Marshal(nodes)
	if e != nil {
		return e
	}
	var items []struct {
		ID     string `json:"id"`
		Source string `json:"source"`
	}
	if e = json.Unmarshal(b, &items); e != nil {
		return e
	}
	var raw []json.RawMessage
	_ = json.Unmarshal(b, &raw)
	tx, e := s.DB.Begin()
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if source != "" {
		if _, e = tx.Exec(`DELETE FROM documents WHERE collection='nodes' AND json_extract(value,'$.source')=?`, source); e != nil {
			return e
		}
	}
	for i, n := range items {
		if _, e = tx.Exec(`INSERT INTO documents VALUES('nodes',?,?) ON CONFLICT(collection,id) DO UPDATE SET value=excluded.value`, n.ID, []byte(raw[i])); e != nil {
			return e
		}
	}
	return tx.Commit()
}
