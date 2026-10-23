package access

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
)

type KeyPage struct {
	Items      []Key  `json:"items"`
	NextCursor string `json:"next_cursor"`
}
type keyCursor struct {
	Created string `json:"created"`
	ID      string `json:"id"`
}

// Keyset pagination has no total count or snapshot guarantee across requests.
// Concurrent creation ahead of the cursor is visible after a refresh, not here.
func (s *Store) KeysPageContext(ctx context.Context, limit int, cursor string) (KeyPage, error) {
	out := KeyPage{Items: []Key{}}
	if err := ctx.Err(); err != nil {
		return out, err
	}
	if limit < 1 || limit > 100 || len(cursor) > 1024 {
		return out, ErrInvalid
	}
	c := keyCursor{}
	if cursor != "" {
		raw, err := base64.RawURLEncoding.DecodeString(cursor)
		if err != nil {
			return out, ErrInvalid
		}
		dec := json.NewDecoder(strings.NewReader(string(raw)))
		dec.DisallowUnknownFields()
		if dec.Decode(&c) != nil || c.Created == "" || c.ID == "" || len(c.Created) > 128 || len(c.ID) > 128 {
			return out, ErrInvalid
		}
		canonical, _ := json.Marshal(c)
		if string(canonical) != string(raw) {
			return out, ErrInvalid
		}
	}
	query := "SELECT " + keyColumns + " FROM api_keys"
	args := []any{}
	if cursor != "" {
		query += " WHERE created_at < ? OR (created_at = ? AND id > ?)"
		args = append(args, c.Created, c.Created, c.ID)
	}
	query += " ORDER BY created_at DESC,id LIMIT ?"
	args = append(args, limit+1)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return KeyPage{}, err
		}
		k, err := scanKey(rows)
		if err != nil {
			return KeyPage{}, err
		}
		out.Items = append(out.Items, k)
	}
	if err := rows.Err(); err != nil {
		return KeyPage{}, err
	}
	if err := ctx.Err(); err != nil {
		return KeyPage{}, err
	}
	if len(out.Items) > limit {
		out.Items = out.Items[:limit]
		last := out.Items[limit-1]
		raw, _ := json.Marshal(keyCursor{last.CreatedAt, last.ID})
		out.NextCursor = base64.RawURLEncoding.EncodeToString(raw)
	}
	return out, nil
}
