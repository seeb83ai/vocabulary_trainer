package db

import (
	"context"
	"database/sql"
	"fmt"
	"vocabulary_trainer/models"
)

// GetComponentHMMSceneText returns the mnemonic scene text for a component character
// for the given user, or "" if none has been saved.
func (s *Store) GetComponentHMMSceneText(ctx context.Context, userID int64, character string) (string, error) {
	var text string
	err := s.db.QueryRowContext(ctx,
		`SELECT scene_text FROM component_hmm_scenes WHERE user_id = ? AND character = ?`,
		userID, character,
	).Scan(&text)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("get component hmm scene: %w", err)
	}
	return text, nil
}

// UpsertComponentHMMScene saves (or replaces) the mnemonic scene text for a
// component character for the given user.
func (s *Store) UpsertComponentHMMScene(ctx context.Context, userID int64, character, sceneText string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO component_hmm_scenes (character, user_id, scene_text) VALUES (?, ?, ?)
		 ON CONFLICT(character, user_id) DO UPDATE SET scene_text = excluded.scene_text`,
		character, userID, sceneText,
	)
	if err != nil {
		return fmt.Errorf("upsert component hmm scene: %w", err)
	}
	return nil
}

// DeleteComponentHMMScene removes the mnemonic scene for a component character
// for the given user. It is a no-op if no scene exists.
func (s *Store) DeleteComponentHMMScene(ctx context.Context, userID int64, character string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM component_hmm_scenes WHERE user_id = ? AND character = ?`,
		userID, character,
	)
	if err != nil {
		return fmt.Errorf("delete component hmm scene: %w", err)
	}
	return nil
}

// GetComponentHMMSceneRecord returns the saved mnemonic scene for a component character
// as a models.HMMScene, or nil if none exists.
func (s *Store) GetComponentHMMSceneRecord(ctx context.Context, userID int64, character string) (*models.HMMScene, error) {
	var text string
	err := s.db.QueryRowContext(ctx,
		`SELECT scene_text FROM component_hmm_scenes WHERE user_id = ? AND character = ?`,
		userID, character,
	).Scan(&text)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get component hmm scene: %w", err)
	}
	return &models.HMMScene{SceneText: text}, nil
}

// SaveComponentHMMSceneWithLibrary saves the mnemonic scene for a component character
// and updates the shared actor/location/room/props library entries.
func (s *Store) SaveComponentHMMSceneWithLibrary(ctx context.Context, userID int64, character, initial, finalKey string, tone int, req models.HMMSaveSceneRequest) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO component_hmm_scenes (user_id, character, scene_text) VALUES (?, ?, ?)
		 ON CONFLICT(user_id, character) DO UPDATE SET scene_text = excluded.scene_text`,
		userID, character, req.SceneText); err != nil {
		return fmt.Errorf("upsert component scene: %w", err)
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
