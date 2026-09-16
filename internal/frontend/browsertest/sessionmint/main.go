// sessionmint is a browser-test utility, never linked into the application.
package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

type session struct {
	Token   string `json:"token"`
	Expires int64  `json:"expires"`
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: sessionmint /absolute/path/to/scratch.db")
		os.Exit(1)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	sess, err := mint(ctx, os.Args[1])
	if err == nil {
		err = json.NewEncoder(os.Stdout).Encode(sess)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func mint(ctx context.Context, path string) (session, error) {
	// No ambient DATABASE_URL, database creation, or migrations: the runner owns setup.
	if !filepath.IsAbs(path) {
		return session{}, fmt.Errorf("scratch database path must be absolute")
	}
	uri := url.URL{Scheme: "file", Path: path}
	db, err := sql.Open("sqlite", uri.String()+"?mode=rw&_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_txlock=immediate")
	if err != nil {
		return session{}, err
	}
	defer db.Close()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return session{}, err
	}
	defer tx.Rollback()
	var entropy [32]byte
	if _, err := rand.Read(entropy[:]); err != nil {
		return session{}, err
	}
	token := hex.EncodeToString(entropy[:])
	userID := "u_browser_" + token
	if _, err := tx.ExecContext(ctx, `INSERT INTO "user" (id, email) VALUES (?, ?)`, userID, "browser-"+token+"@example.com"); err != nil {
		return session{}, err
	}
	expires := time.Now().UTC().Add(time.Hour).Truncate(time.Second)
	if _, err := tx.ExecContext(ctx, `INSERT INTO session (token, user_id, expires_at) VALUES (?, ?, ?)`, token, userID, expires.Format(time.RFC3339)); err != nil {
		return session{}, err
	}
	if err := tx.Commit(); err != nil {
		return session{}, err
	}
	return session{Token: token, Expires: expires.Unix()}, nil
}
