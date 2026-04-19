package migrate

import (
	"database/sql"
	"fmt"
)

func init() {
	register(migration{
		version: 7,
		fn: func(db *sql.DB) error {
			for _, col := range []string{"words_known", "new_words"} {
				var count int
				if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('daily_stats') WHERE name = ?`, col).Scan(&count); err != nil {
					return fmt.Errorf("check %s column: %w", col, err)
				}
				if count > 0 {
					if _, err := db.Exec(fmt.Sprintf(`ALTER TABLE daily_stats DROP COLUMN %s`, col)); err != nil {
						return fmt.Errorf("drop %s column: %w", col, err)
					}
				}
			}
			return nil
		},
	})
}
