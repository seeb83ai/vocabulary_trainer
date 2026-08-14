package migrate

// v20260814165558 adds a plain (non-partial) index on
// hanzi_decomposition_translation(character, lang). The existing indexes from
// v59 are partial (WHERE user_id IS NULL / WHERE user_id IS NOT NULL) to
// enforce the copy-on-write uniqueness constraints, so the query planner can't
// use either of them for a LEFT JOIN ... ON character = ? AND lang = ? that
// doesn't filter on user_id — which is exactly the join GetComponentList uses
// to search components by definition. This index covers that join directly.
func init() {
	register(migration{
		version: 20260814165558,
		sql:     `CREATE INDEX IF NOT EXISTS idx_hanzi_trans_char_lang ON hanzi_decomposition_translation(character, lang);`,
	})
}
