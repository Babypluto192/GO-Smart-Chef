package main

import (
	"database/sql"
	"time"
)

type User struct {
	ID           int    `json:"id"`
	Username     string `json:"username"`
	PasswordHash string `json:"-"`
	Preferences  string `json:"preferences"`
}

type RecipeRecord struct {
	ID        int       `json:"id"`
	UserID    int       `json:"user_id"`
	QueryType string    `json:"query_type"`
	QueryText string    `json:"query_text"`
	Result    string    `json:"result"`
	CreatedAt time.Time `json:"created_at"`
}

type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) InitSchema() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT UNIQUE NOT NULL,
			password_hash TEXT NOT NULL,
			preferences TEXT DEFAULT ''
		);`,
		`CREATE TABLE IF NOT EXISTS user_history (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			query_type TEXT NOT NULL,
			query_text TEXT NOT NULL,
			result TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS daily_recipes (
			date TEXT PRIMARY KEY,
			recipe TEXT NOT NULL
		);`,
	}

	for _, q := range queries {
		if _, err := r.db.Exec(q); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) CreateUser(username, passwordHash, preferences string) (int64, error) {
	res, err := r.db.Exec(
		`INSERT INTO users (username, password_hash, preferences) VALUES (?, ?, ?)`,
		username, passwordHash, preferences,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (r *Repository) GetUserByUsername(username string) (*User, error) {
	u := &User{}
	err := r.db.QueryRow(
		`SELECT id, username, password_hash, preferences FROM users WHERE username = ?`,
		username,
	).Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Preferences)
	if err != nil {
		return nil, err
	}
	return u, nil
}

func (r *Repository) GetUserByID(id int) (*User, error) {
	u := &User{}
	err := r.db.QueryRow(
		`SELECT id, username, password_hash, preferences FROM users WHERE id = ?`,
		id,
	).Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Preferences)
	if err != nil {
		return nil, err
	}
	return u, nil
}

func (r *Repository) SaveHistory(userID int, queryType, queryText, result string) error {
	_, err := r.db.Exec(
		`INSERT INTO user_history (user_id, query_type, query_text, result) VALUES (?, ?, ?, ?)`,
		userID, queryType, queryText, result,
	)
	return err
}

func (r *Repository) GetUserRecentQueries(userID int, limit int) ([]string, error) {
	rows, err := r.db.Query(
		`SELECT query_text FROM user_history WHERE user_id = ? ORDER BY created_at DESC LIMIT ?`,
		userID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var queries []string
	for rows.Next() {
		var q string
		if err := rows.Scan(&q); err == nil {
			queries = append(queries, q)
		}
	}
	return queries, nil
}

func (r *Repository) GetDailyRecipe(date string) (string, error) {
	var recipe string
	err := r.db.QueryRow(`SELECT recipe FROM daily_recipes WHERE date = ?`, date).Scan(&recipe)
	return recipe, err
}

func (r *Repository) SaveDailyRecipe(date, recipe string) error {
	_, err := r.db.Exec(`INSERT OR REPLACE INTO daily_recipes (date, recipe) VALUES (?, ?)`, date, recipe)
	return err
}
