package repo

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/wearwhere/wearwhere_be/internal/wardrobe/domain"
)

type SnapshotPG struct{ db DBTX }

func NewSnapshotPG(db DBTX) *SnapshotPG { return &SnapshotPG{db: db} }

func (r *SnapshotPG) Load(ctx context.Context, userID uuid.UUID) (*Snapshot, error) {
	var sig string
	var raw []byte
	err := r.db.QueryRow(ctx,
		`SELECT signature, outfits FROM wardrobe_snapshots WHERE user_id = $1`, userID).
		Scan(&sig, &raw)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNoSnapshot
		}
		return nil, err
	}
	var outfits []domain.Outfit
	if err := json.Unmarshal(raw, &outfits); err != nil {
		return nil, err
	}
	return &Snapshot{Signature: sig, Outfits: outfits}, nil
}

func (r *SnapshotPG) Upsert(ctx context.Context, userID uuid.UUID, sig string, outfits []domain.Outfit, model string, tokensIn, tokensOut int) error {
	raw, err := json.Marshal(outfits)
	if err != nil {
		return err
	}
	_, err = r.db.Exec(ctx, `
		INSERT INTO wardrobe_snapshots (user_id, signature, outfits, model, tokens_in, tokens_out, generated_at)
		VALUES ($1, $2, $3, $4, $5, $6, NOW())
		ON CONFLICT (user_id) DO UPDATE
		  SET signature = EXCLUDED.signature,
		      outfits = EXCLUDED.outfits,
		      model = EXCLUDED.model,
		      tokens_in = EXCLUDED.tokens_in,
		      tokens_out = EXCLUDED.tokens_out,
		      generated_at = NOW()`,
		userID, sig, raw, model, tokensIn, tokensOut)
	return err
}
