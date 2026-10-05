-- Checks the one-time conversion to library references (ADR-0005).
-- Run it on the converted database with the automatic backup attached as "old":
--
--   sqlite3 -readonly data/verify.db \
--     "ATTACH 'data/verify.db.pre-library-refs' AS old" \
--     ".read scripts/verify-library-conversion.sql"
--
-- Expected: query 2 shows 0 words_changed for users with faithful = 1 and
-- 0 glosses_lost for everybody; queries 3-5 return no rows.
.headers on
.mode column

-- 1. Converted words per user (word ids do not change)
SELECT w.user_id,
       COUNT(*)                          AS zh_words,
       SUM(w.library_word_id IS NOT NULL) AS references_now
FROM main.words w
WHERE w.language = 'zh' AND w.user_id != 1
GROUP BY w.user_id ORDER BY w.user_id;

-- 2. Translations before vs. after, per user.
--    faithful = 1: the user trained in the last 7 days before the conversion,
--    so every word must look exactly the same (words_changed = 0).
--    Everybody else may get more library translations, but loses none.
WITH old_g AS (
  SELECT w.user_id, w.id, tw.language AS lang, tw.text
  FROM old.words w
  JOIN old.translations t ON t.zh_word_id = w.id
  JOIN old.words tw ON tw.id = t.translation_word_id
  WHERE w.language = 'zh' AND w.user_id != 1
),
new_g AS (
  SELECT w.user_id, w.id, tw.language AS lang, tw.text
  FROM main.words w
  JOIN main.user_translations t ON t.zh_word_id = w.id
  JOIN main.words tw ON tw.id = t.translation_word_id
  WHERE w.language = 'zh' AND w.user_id != 1
),
lost_rows AS (
  SELECT o.* FROM old_g o
  WHERE NOT EXISTS (SELECT 1 FROM new_g n WHERE n.id = o.id AND n.lang = o.lang AND n.text = o.text)
),
added_rows AS (
  SELECT n.* FROM new_g n
  WHERE NOT EXISTS (SELECT 1 FROM old_g o WHERE o.id = n.id AND o.lang = n.lang AND o.text = n.text)
),
faithful AS (
  SELECT user_id FROM old.daily_stats WHERE date >= date((SELECT done_at FROM main.data_conversions WHERE name = 'library_references'), '-7 days')
  UNION
  SELECT user_id FROM old.usage_events WHERE substr(last_seen, 1, 10) >= date((SELECT done_at FROM main.data_conversions WHERE name = 'library_references'), '-7 days')
)
SELECT u.user_id,
       u.user_id IN (SELECT user_id FROM faithful)                              AS faithful,
       (SELECT COUNT(*) FROM lost_rows l WHERE l.user_id = u.user_id)           AS glosses_lost,
       (SELECT COUNT(*) FROM added_rows a WHERE a.user_id = u.user_id)          AS glosses_added,
       (SELECT COUNT(DISTINCT id) FROM (SELECT user_id, id FROM lost_rows UNION ALL SELECT user_id, id FROM added_rows) x
         WHERE x.user_id = u.user_id)                                           AS words_changed
FROM (SELECT DISTINCT user_id FROM old_g) u
ORDER BY u.user_id;

-- 3. Words that disappeared (must be empty)
SELECT o.user_id, o.id, o.text FROM old.words o
WHERE o.language = 'zh' AND o.user_id != 1
  AND NOT EXISTS (SELECT 1 FROM main.words n WHERE n.id = o.id AND n.text = o.text);

-- 4. Progress that changed (must be empty)
SELECT o.word_id FROM old.sm2_progress o
JOIN old.words w ON w.id = o.word_id AND w.language = 'zh' AND w.user_id != 1
JOIN main.sm2_progress n ON n.word_id = o.word_id
WHERE n.repetitions IS NOT o.repetitions OR n.due_date IS NOT o.due_date
   OR n.total_attempts IS NOT o.total_attempts OR n.total_correct IS NOT o.total_correct
   OR n.is_known IS NOT o.is_known;

-- 5. Tags that changed (must be empty)
SELECT word_id, tag_id FROM (
  SELECT wt.word_id, wt.tag_id FROM old.word_tags wt JOIN old.words w ON w.id = wt.word_id AND w.language = 'zh' AND w.user_id != 1
  EXCEPT
  SELECT wt.word_id, wt.tag_id FROM main.word_tags wt JOIN main.words w ON w.id = wt.word_id AND w.language = 'zh' AND w.user_id != 1
);
