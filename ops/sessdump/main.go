// Ad hoc prod-DB inspector: lists sessions (email + expiry) from a copied SQLite file.
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
	rows, err := db.Query(`SELECT u.email, s.expires_at FROM session s JOIN "user" u ON u.id = s.user_id ORDER BY s.expires_at`)
	if err != nil {
		log.Fatal(err)
	}
	for rows.Next() {
		var email, exp string
		if err := rows.Scan(&email, &exp); err != nil {
			log.Fatal(err)
		}
		fmt.Println(email, "expires", exp)
	}
}
