package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

// Console users (migration 0016, artex_users). The legacy bootstrap login
// "ARTEX" is not a row here: it lives in settings and stays the break-glass
// admin. Every write runs in a transaction that first sets artex.actor, so the
// audit trigger on artex_users records who made the change. Password hashes
// are computed by the caller (bcrypt) and never leave this package except
// through UserAuth, which login uses.

// User roles, matching the CHECK constraint on artex_users.role.
const (
	RoleAdmin      = "admin"
	RoleOperator   = "operator"
	RoleReviewer   = "reviewer"
	RoleClientRead = "client_read"
)

// UserRoles lists every valid role, most privileged first.
var UserRoles = []string{RoleAdmin, RoleOperator, RoleReviewer, RoleClientRead}

// ValidUserRole reports whether role is one of UserRoles.
func ValidUserRole(role string) bool {
	for _, r := range UserRoles {
		if r == role {
			return true
		}
	}
	return false
}

var (
	ErrUserNotFound = errors.New("user not found")
	ErrUserExists   = errors.New("username already exists")
)

// User is a console user without its password hash.
type User struct {
	ID        int64     `json:"id"`
	Username  string    `json:"username"`
	Role      string    `json:"role"`
	Disabled  bool      `json:"disabled"`
	Version   int64     `json:"version"`
	CreatedBy string    `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// UserAuth is a user with its password hash, for login only.
type UserAuth struct {
	User
	PasswordHash string
}

// UserPatch changes some fields of a user. Nil fields are left alone. Any
// applied patch bumps the user's version, which revokes its sessions.
type UserPatch struct {
	Role         *string
	Disabled     *bool
	PasswordHash *string
}

func (p UserPatch) empty() bool {
	return p.Role == nil && p.Disabled == nil && p.PasswordHash == nil
}

const userCols = `id, username, role, disabled, version, created_by, created_at, updated_at`

type rowScanner interface{ Scan(dest ...any) error }

func scanUser(r rowScanner, extra ...any) (User, error) {
	var u User
	dest := append([]any{&u.ID, &u.Username, &u.Role, &u.Disabled, &u.Version, &u.CreatedBy, &u.CreatedAt, &u.UpdatedAt}, extra...)
	err := r.Scan(dest...)
	return u, err
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// withActor runs fn in a transaction whose artex.actor is actor.
func (d *DB) withActor(actor string, fn func(*sql.Tx) error) error {
	tx, err := d.BeginTx(context.Background(), nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	if _, err := tx.Exec(`SELECT set_config('artex.actor', $1, true)`, actor); err != nil {
		return fmt.Errorf("set audit actor: %w", err)
	}
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}

// CreateUser inserts a user. username is stored lower-cased.
func (d *DB) CreateUser(actor, username, passwordHash, role string) (User, error) {
	var u User
	err := d.withActor(actor, func(tx *sql.Tx) error {
		var err error
		u, err = scanUser(tx.QueryRow(`INSERT INTO artex_users(username, password_hash, role, created_by)
			VALUES ($1, $2, $3, $4) RETURNING `+userCols,
			strings.ToLower(username), passwordHash, role, actor))
		return err
	})
	if isUniqueViolation(err) {
		return User{}, ErrUserExists
	}
	return u, err
}

// GetUser returns the user with id, or ErrUserNotFound.
func (d *DB) GetUser(id int64) (User, error) {
	u, err := scanUser(d.QueryRow(`SELECT `+userCols+` FROM artex_users WHERE id = $1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrUserNotFound
	}
	return u, err
}

// GetUserAuth returns the user named username (lower-cased) with its password
// hash, or ErrUserNotFound.
func (d *DB) GetUserAuth(username string) (UserAuth, error) {
	var a UserAuth
	u, err := scanUser(d.QueryRow(`SELECT `+userCols+`, password_hash FROM artex_users WHERE username = $1`,
		strings.ToLower(username)), &a.PasswordHash)
	if errors.Is(err, sql.ErrNoRows) {
		return UserAuth{}, ErrUserNotFound
	}
	a.User = u
	return a, err
}

// ListUsers returns every user, without hashes, ordered by username.
func (d *DB) ListUsers() ([]User, error) {
	rows, err := d.Query(`SELECT ` + userCols + ` FROM artex_users ORDER BY username`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []User{}
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// UpdateUser applies p to the user with id and bumps its version. An empty
// patch returns the user unchanged.
func (d *DB) UpdateUser(actor string, id int64, p UserPatch) (User, error) {
	if p.empty() {
		return d.GetUser(id)
	}
	var u User
	err := d.withActor(actor, func(tx *sql.Tx) error {
		var err error
		u, err = scanUser(tx.QueryRow(`UPDATE artex_users SET
				role = COALESCE($2, role),
				disabled = COALESCE($3, disabled),
				password_hash = COALESCE($4, password_hash),
				version = version + 1
			WHERE id = $1 RETURNING `+userCols, id, p.Role, p.Disabled, p.PasswordHash))
		return err
	})
	d.invalidateUser(id)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrUserNotFound
	}
	return u, err
}

// DeleteUser removes the user with id, or returns ErrUserNotFound.
func (d *DB) DeleteUser(actor string, id int64) error {
	err := d.withActor(actor, func(tx *sql.Tx) error {
		res, err := tx.Exec(`DELETE FROM artex_users WHERE id = $1`, id)
		if err != nil {
			return err
		}
		if n, err := res.RowsAffected(); err != nil {
			return err
		} else if n == 0 {
			return ErrUserNotFound
		}
		return nil
	})
	d.invalidateUser(id)
	return err
}
