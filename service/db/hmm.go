package db

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"vocabulary_trainer/models"
)

func (s *Store) GetHMMActors(ctx context.Context, userID int64) ([]models.HMMActor, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT initial, category, actor_name, hint FROM hmm_actors
		 WHERE user_id = ?
		 ORDER BY CASE category
		   WHEN 'male' THEN 1 WHEN 'female' THEN 2
		   WHEN 'fictional' THEN 3 WHEN 'wildcard' THEN 4 END, initial`, userID)
	if err != nil {
		return nil, fmt.Errorf("get hmm actors: %w", err)
	}
	var actors []models.HMMActor
	for rows.Next() {
		var a models.HMMActor
		if err := rows.Scan(&a.Initial, &a.Category, &a.ActorName, &a.Hint); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan hmm actor: %w", err)
		}
		actors = append(actors, a)
	}
	rows.Close()
	return actors, rows.Err()
}

func (s *Store) UpdateHMMActor(ctx context.Context, userID int64, initial, actorName string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE hmm_actors SET actor_name = ? WHERE user_id = ? AND initial = ?`, actorName, userID, initial)
	if err != nil {
		return fmt.Errorf("update hmm actor: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("hmm actor %q not found", initial)
	}
	return s.ensureHMMProgressRow(ctx, userID, "actor", initial, actorName)
}

func (s *Store) GetHMMLocations(ctx context.Context, userID int64) ([]models.HMMLocation, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT final_key, location_name FROM hmm_locations WHERE user_id = ? ORDER BY final_key`, userID)
	if err != nil {
		return nil, fmt.Errorf("get hmm locations: %w", err)
	}
	var locs []models.HMMLocation
	for rows.Next() {
		var l models.HMMLocation
		if err := rows.Scan(&l.FinalKey, &l.LocationName); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan hmm location: %w", err)
		}
		locs = append(locs, l)
	}
	rows.Close()
	return locs, rows.Err()
}

func (s *Store) UpdateHMMLocation(ctx context.Context, userID int64, finalKey, locationName string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE hmm_locations SET location_name = ? WHERE user_id = ? AND final_key = ?`, locationName, userID, finalKey)
	if err != nil {
		return fmt.Errorf("update hmm location: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("hmm location %q not found", finalKey)
	}
	return s.ensureHMMProgressRow(ctx, userID, "location", finalKey, locationName)
}

func (s *Store) GetHMMToneRooms(ctx context.Context, userID int64) ([]models.HMMToneRoom, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT tone, room_name FROM hmm_tone_rooms WHERE user_id = ? ORDER BY tone`, userID)
	if err != nil {
		return nil, fmt.Errorf("get hmm tone rooms: %w", err)
	}
	var rooms []models.HMMToneRoom
	for rows.Next() {
		var tr models.HMMToneRoom
		if err := rows.Scan(&tr.Tone, &tr.RoomName); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan hmm tone room: %w", err)
		}
		rooms = append(rooms, tr)
	}
	rows.Close()
	return rooms, rows.Err()
}

func (s *Store) UpdateHMMToneRoom(ctx context.Context, userID int64, tone int, roomName string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE hmm_tone_rooms SET room_name = ? WHERE user_id = ? AND tone = ?`, roomName, userID, tone)
	if err != nil {
		return fmt.Errorf("update hmm tone room: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("hmm tone room %d not found", tone)
	}
	return s.ensureHMMProgressRow(ctx, userID, "tone_room", strconv.Itoa(tone), roomName)
}

func (s *Store) GetHMMProps(ctx context.Context, userID int64) ([]models.HMMProp, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT radical, prop_name FROM hmm_props WHERE user_id = ? ORDER BY radical`, userID)
	if err != nil {
		return nil, fmt.Errorf("get hmm props: %w", err)
	}
	var props []models.HMMProp
	for rows.Next() {
		var p models.HMMProp
		if err := rows.Scan(&p.Radical, &p.PropName); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan hmm prop: %w", err)
		}
		props = append(props, p)
	}
	rows.Close()
	return props, rows.Err()
}

func (s *Store) UpsertHMMProp(ctx context.Context, userID int64, radical, propName string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO hmm_props (user_id, radical, prop_name) VALUES (?, ?, ?)
		 ON CONFLICT(user_id, radical) DO UPDATE SET prop_name = excluded.prop_name`,
		userID, radical, propName)
	if err != nil {
		return fmt.Errorf("upsert hmm prop: %w", err)
	}
	return s.ensureHMMProgressRow(ctx, userID, "prop", radical, propName)
}

func (s *Store) DeleteHMMProp(ctx context.Context, userID int64, radical string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM hmm_props WHERE user_id = ? AND radical = ?`, userID, radical)
	if err != nil {
		return fmt.Errorf("delete hmm prop: %w", err)
	}
	return nil
}

func (s *Store) GetHMMScene(ctx context.Context, wordID int64) (*models.HMMScene, error) {
	var sc models.HMMScene
	err := s.db.QueryRowContext(ctx,
		`SELECT word_id, scene_text FROM hmm_scenes WHERE word_id = ?`, wordID).
		Scan(&sc.WordID, &sc.SceneText)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get hmm scene: %w", err)
	}
	return &sc, nil
}

func (s *Store) UpsertHMMScene(ctx context.Context, wordID int64, sceneText string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO hmm_scenes (word_id, scene_text) VALUES (?, ?)
		 ON CONFLICT(word_id) DO UPDATE SET scene_text = excluded.scene_text`,
		wordID, sceneText)
	if err != nil {
		return fmt.Errorf("upsert hmm scene: %w", err)
	}
	return nil
}

func (s *Store) DeleteHMMScene(ctx context.Context, userID, wordID int64) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM hmm_scenes WHERE word_id = ?
		 AND word_id IN (SELECT id FROM words WHERE user_id = ?)`,
		wordID, userID)
	if err != nil {
		return fmt.Errorf("delete hmm scene: %w", err)
	}
	return nil
}

func (s *Store) GetHMMSceneText(ctx context.Context, wordID int64) (string, error) {
	var text string
	err := s.db.QueryRowContext(ctx,
		`SELECT scene_text FROM hmm_scenes WHERE word_id = ?`, wordID).Scan(&text)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("get hmm scene text: %w", err)
	}
	return text, nil
}

func (s *Store) GetHMMActorByInitial(ctx context.Context, userID int64, initial string) (*models.HMMActor, error) {
	var a models.HMMActor
	err := s.db.QueryRowContext(ctx,
		`SELECT initial, category, actor_name, hint FROM hmm_actors WHERE user_id = ? AND initial = ?`, userID, initial).
		Scan(&a.Initial, &a.Category, &a.ActorName, &a.Hint)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get hmm actor by initial: %w", err)
	}
	return &a, nil
}

func (s *Store) GetHMMLocationByFinal(ctx context.Context, userID int64, finalKey string) (*models.HMMLocation, error) {
	var l models.HMMLocation
	err := s.db.QueryRowContext(ctx,
		`SELECT final_key, location_name FROM hmm_locations WHERE user_id = ? AND final_key = ?`, userID, finalKey).
		Scan(&l.FinalKey, &l.LocationName)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get hmm location by final: %w", err)
	}
	return &l, nil
}

func (s *Store) GetHMMToneRoom(ctx context.Context, userID int64, tone int) (*models.HMMToneRoom, error) {
	var tr models.HMMToneRoom
	err := s.db.QueryRowContext(ctx,
		`SELECT tone, room_name FROM hmm_tone_rooms WHERE user_id = ? AND tone = ?`, userID, tone).
		Scan(&tr.Tone, &tr.RoomName)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get hmm tone room: %w", err)
	}
	return &tr, nil
}

func (s *Store) GetHMMPropsByRadicals(ctx context.Context, userID int64, radicals []string) ([]models.HMMProp, error) {
	if len(radicals) == 0 {
		return nil, nil
	}
	placeholders := make([]string, len(radicals))
	args := make([]any, len(radicals)+1)
	args[0] = userID
	for i, r := range radicals {
		placeholders[i] = "?"
		args[i+1] = r
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT radical, prop_name FROM hmm_props WHERE user_id = ? AND radical IN (`+strings.Join(placeholders, ",")+`)`,
		args...)
	if err != nil {
		return nil, fmt.Errorf("get hmm props by radicals: %w", err)
	}
	var props []models.HMMProp
	for rows.Next() {
		var p models.HMMProp
		if err := rows.Scan(&p.Radical, &p.PropName); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan hmm prop: %w", err)
		}
		props = append(props, p)
	}
	rows.Close()
	return props, rows.Err()
}

func (s *Store) SaveHMMSceneWithLibrary(ctx context.Context, userID, wordID int64, initial, finalKey string, tone int, req models.HMMSaveSceneRequest) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO hmm_scenes (word_id, scene_text) VALUES (?, ?)
		 ON CONFLICT(word_id) DO UPDATE SET scene_text = excluded.scene_text`,
		wordID, req.SceneText); err != nil {
		return fmt.Errorf("upsert scene: %w", err)
	}

	if req.ActorName != "" && initial != "" {
		if _, err := tx.ExecContext(ctx,
			`UPDATE hmm_actors SET actor_name = ? WHERE user_id = ? AND initial = ?`,
			req.ActorName, userID, initial); err != nil {
			return fmt.Errorf("update actor: %w", err)
		}
	}
	if req.LocationName != "" && finalKey != "" {
		if _, err := tx.ExecContext(ctx,
			`UPDATE hmm_locations SET location_name = ? WHERE user_id = ? AND final_key = ?`,
			req.LocationName, userID, finalKey); err != nil {
			return fmt.Errorf("update location: %w", err)
		}
	}
	if req.RoomName != "" && tone >= 1 && tone <= 5 {
		if _, err := tx.ExecContext(ctx,
			`UPDATE hmm_tone_rooms SET room_name = ? WHERE user_id = ? AND tone = ?`,
			req.RoomName, userID, tone); err != nil {
			return fmt.Errorf("update tone room: %w", err)
		}
	}
	for _, p := range req.Props {
		if p.Radical == "" {
			continue
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO hmm_props (user_id, radical, prop_name) VALUES (?, ?, ?)
			 ON CONFLICT(user_id, radical) DO UPDATE SET prop_name = excluded.prop_name`,
			userID, p.Radical, p.PropName); err != nil {
			return fmt.Errorf("upsert prop %s: %w", p.Radical, err)
		}
	}

	return tx.Commit()
}
