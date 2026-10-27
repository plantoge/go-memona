package access

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

// User is an account authenticated through Google's stable subject identifier.
type User struct {
	ID        string `json:"id"`
	Email     string `json:"email"`
	Name      string `json:"name"`
	GoogleSub string `json:"google_sub"`
}

func (s *Store) initUsers() error {
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS users (
 id TEXT PRIMARY KEY, google_sub TEXT NOT NULL UNIQUE, email TEXT NOT NULL, name TEXT NOT NULL);
 CREATE TABLE IF NOT EXISTS user_sessions (
 hash TEXT PRIMARY KEY, user_id TEXT NOT NULL REFERENCES users(id), csrf TEXT NOT NULL, expires INTEGER NOT NULL);`)
	return err
}

// UserLoginContext upserts only by Google subject; email is mutable and never an identity key.
func (s *Store) UserLoginContext(ctx context.Context, sub, email, name string) (User, string, string, error) {
	if err := ctx.Err(); err != nil {
		return User{}, "", "", err
	}
	if strings.TrimSpace(sub) != sub || sub == "" || len(sub) > 255 || email == "" || len(email) > 320 || len(name) > 512 || strings.ContainsAny(sub+email+name, "\x00\r\n") {
		return User{}, "", "", ErrInvalid
	}
	id, err := Random()
	if err != nil {
		return User{}, "", "", err
	}
	token, err := Random()
	if err != nil {
		return User{}, "", "", err
	}
	csrf, err := Random()
	if err != nil {
		return User{}, "", "", err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return User{}, "", "", requestContextError(ctx, err)
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT INTO users(id,google_sub,email,name) VALUES(?,?,?,?)
 ON CONFLICT(google_sub) DO UPDATE SET email=excluded.email,name=excluded.name`, id, sub, email, name)
	if err != nil {
		return User{}, "", "", requestContextError(ctx, err)
	}
	var user User
	err = tx.QueryRowContext(ctx, "SELECT id,email,name,google_sub FROM users WHERE google_sub=?", sub).Scan(&user.ID, &user.Email, &user.Name, &user.GoogleSub)
	if err != nil {
		return User{}, "", "", requestContextError(ctx, err)
	}
	_, err = tx.ExecContext(ctx, "INSERT INTO user_sessions(hash,user_id,csrf,expires) VALUES(?,?,?,?)", digest(token), user.ID, csrf, time.Now().Add(8*time.Hour).Unix())
	if err != nil {
		return User{}, "", "", requestContextError(ctx, err)
	}
	if err = tx.Commit(); err != nil {
		return User{}, "", "", requestContextError(ctx, err)
	}
	return user, token, csrf, nil
}

func (s *Store) UserSessionContext(ctx context.Context, token string) (User, string, error) {
	if err := ctx.Err(); err != nil {
		return User{}, "", err
	}
	if len(token) != 64 {
		return User{}, "", ErrDenied
	}
	var u User
	var csrf string
	var expires int64
	err := s.db.QueryRowContext(ctx, `SELECT u.id,u.email,u.name,u.google_sub,us.csrf,us.expires FROM user_sessions us JOIN users u ON u.id=us.user_id WHERE us.hash=?`, digest(token)).Scan(&u.ID, &u.Email, &u.Name, &u.GoogleSub, &csrf, &expires)
	if err != nil {
		if ctx.Err() != nil {
			return User{}, "", ctx.Err()
		}
		if errors.Is(err, sql.ErrNoRows) {
			return User{}, "", ErrDenied
		}
		return User{}, "", err
	}
	if err := ctx.Err(); err != nil {
		return User{}, "", err
	}
	if time.Now().Unix() >= expires {
		return User{}, "", ErrDenied
	}
	return u, csrf, nil
}

func (s *Store) UserLogoutContext(ctx context.Context, token string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(token) != 64 {
		return ErrDenied
	}
	_, err := s.db.ExecContext(ctx, "DELETE FROM user_sessions WHERE hash=?", digest(token))
	return err
}
