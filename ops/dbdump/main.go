// Ad hoc prod-DB inspector: dumps auth-related tables from a copied SQLite file.
package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"

	_ "modernc.org/sqlite"
)

func main() {
	db, err := sql.Open("sqlite", "file:"+os.Args[1]+"?mode=ro")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	for _, q := range []struct{ label, query string }{
		{"suppression", `SELECT email, reason, detail, created_at FROM suppression ORDER BY created_at`},
		{"login_code", `SELECT email, user_id IS NOT NULL, attempts, created_at, expires_at FROM login_code ORDER BY created_at`},
		{"user", `SELECT id, email, created_at FROM "user" ORDER BY created_at`},
	} {
		fmt.Printf("== %s ==\n", q.label)
		rows, err := db.Query(q.query)
		if err != nil {
			fmt.Printf("  query error: %v\n", err)
			continue
		}
		cols, _ := rows.Columns()
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		n := 0
		for rows.Next() {
			if err := rows.Scan(ptrs...); err != nil {
				log.Fatal(err)
			}
			for i, v := range vals {
				if b, ok := v.([]byte); ok {
					vals[i] = string(b)
				}
			}
			fmt.Printf("  %v\n", vals)
			n++
		}
		fmt.Printf("  (%d rows)\n", n)
	}
}
