package migrate

import (
	"database/sql"
	"fmt"
)

// v20260802103000: generalize confusion_pairs so a "confusion" side can be
// either a vocabulary word (zh_word_id) or a hanzi component (zh_component,
// keyed by character text) — needed so component quiz answers (issue #280)
// can be tracked the same way word answers already are.
//
// zh_word_id/confused_with_id lose their words(id) FK and switch from
// "always a real id" to "0 when this side is a component" so that
// ON CONFLICT dedup keeps working (SQLite never treats two NULLs as equal in
// a PK, which would silently break dedup for component-only pairs).
// user_id is added directly because component characters, unlike word ids,
// are not inherently user-scoped. DeleteWord now cleans up its own
// confusion_pairs rows explicitly since the FK cascade is gone.
func init() {
	register(migration{
		version: 20260802103000,
		fn: func(db *sql.DB) error {
			var count int
			if err := db.QueryRow(
				`SELECT COUNT(*) FROM pragma_table_info('confusion_pairs') WHERE name = 'zh_component'`,
			).Scan(&count); err != nil {
				return fmt.Errorf("check zh_component column: %w", err)
			}
			if count > 0 {
				return nil // already migrated
			}

			for _, stmt := range []string{
				`CREATE TABLE confusion_pairs_new (
				  user_id                 INTEGER  NOT NULL REFERENCES users(id) ON DELETE CASCADE,
				  zh_word_id              INTEGER  NOT NULL DEFAULT 0,
				  zh_component            TEXT     NOT NULL DEFAULT '',
				  confused_with_id        INTEGER  NOT NULL DEFAULT 0,
				  confused_with_component TEXT     NOT NULL DEFAULT '',
				  mode                    TEXT     NOT NULL,
				  count                   INTEGER  NOT NULL DEFAULT 1,
				  last_seen               DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
				  last_shown_in_game      TEXT,
				  PRIMARY KEY (user_id, zh_word_id, zh_component, confused_with_id, confused_with_component, mode)
				)`,
				`INSERT INTO confusion_pairs_new
				   (user_id, zh_word_id, confused_with_id, mode, count, last_seen, last_shown_in_game)
				 SELECT wz.user_id, cp.zh_word_id, cp.confused_with_id, cp.mode, cp.count, cp.last_seen, cp.last_shown_in_game
				 FROM confusion_pairs cp
				 JOIN words wz ON wz.id = cp.zh_word_id`,
				`DROP TABLE confusion_pairs`,
				`ALTER TABLE confusion_pairs_new RENAME TO confusion_pairs`,
			} {
				if _, err := db.Exec(stmt); err != nil {
					return fmt.Errorf("generalize confusion_pairs: %w", err)
				}
			}
			return nil
		},
	})
}
