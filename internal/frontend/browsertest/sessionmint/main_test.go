package main

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/GVPproj/unbusy.day/internal/auth"
	"github.com/GVPproj/unbusy.day/internal/migrate"
)

func TestMintFreshSessions(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "smoke.db")
	if err := migrate.Run(ctx, path); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	svc := auth.NewService(db, nil)
	owners := map[string]bool{}
	for range 3 {
		sess, err := mint(ctx, path)
		if err != nil {
			t.Fatal(err)
		}
		owner, err := svc.UserForSession(ctx, sess.Token)
		if err != nil {
			t.Fatal(err)
		}
		if owners[owner] {
			t.Fatal("reused owner")
		}
		owners[owner] = true
		if len(sess.Token) != 64 || sess.Expires <= time.Now().Unix() {
			t.Fatalf("invalid session: %+v", sess)
		}
		var start, end, blocks int
		var jot string
		if err := db.QueryRow(`SELECT day_start, day_end, jot FROM "user" WHERE id = ?`, owner).Scan(&start, &end, &jot); err != nil {
			t.Fatal(err)
		}
		if err := db.QueryRow(`SELECT count(*) FROM block WHERE owner_id = ?`, owner).Scan(&blocks); err != nil {
			t.Fatal(err)
		}
		if start != 18 || end != 34 || jot != "" || blocks != 0 {
			t.Fatal("user is not fresh")
		}
	}
	var codes int
	if err := db.QueryRow(`SELECT count(*) FROM login_code`).Scan(&codes); err != nil {
		t.Fatal(err)
	}
	if codes != 0 {
		t.Fatal("mint touched OTP flow")
	}
}

func TestMintRequiresExistingAbsoluteDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.db")
	t.Setenv("DATABASE_URL", path)
	for _, path := range []string{"", "relative.db", path} {
		if _, err := mint(context.Background(), path); err == nil {
			t.Fatalf("accepted %q", path)
		}
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("created missing database: %v", err)
	}
}

func TestMintRollsBackUserOnSessionFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "broken.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE "user" (id TEXT PRIMARY KEY, email TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if _, err := mint(context.Background(), path); err == nil {
		t.Fatal("accepted missing session table")
	}
	var users int
	if err := db.QueryRow(`SELECT count(*) FROM "user"`).Scan(&users); err != nil {
		t.Fatal(err)
	}
	if users != 0 {
		t.Fatal("left an orphan user")
	}
}
