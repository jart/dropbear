package auth

import (
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"time"
)

type Invite struct {
	ID        int64
	Token     string
	CreatedBy int64
	UsedBy    *int64
	CreatedAt time.Time
	UsedAt    *time.Time
}

func CreateInvite(db *sql.DB, createdBy int64) (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	token := base64.URLEncoding.EncodeToString(b)

	_, err := db.Exec(
		"INSERT INTO invites (token, created_by) VALUES (?, ?)",
		token, createdBy,
	)
	if err != nil {
		return "", err
	}
	return token, nil
}

func GetInvite(db *sql.DB, token string) (*Invite, error) {
	inv := &Invite{}
	err := db.QueryRow(
		"SELECT id, token, created_by, used_by, created_at, used_at FROM invites WHERE token = ?",
		token,
	).Scan(&inv.ID, &inv.Token, &inv.CreatedBy, &inv.UsedBy, &inv.CreatedAt, &inv.UsedAt)
	if err != nil {
		return nil, err
	}
	return inv, nil
}

func UseInvite(db *sql.DB, token string, usedBy int64) error {
	result, err := db.Exec(
		"UPDATE invites SET used_by = ?, used_at = ? WHERE token = ? AND used_by IS NULL",
		usedBy, time.Now(), token,
	)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func ListInvites(db *sql.DB, createdBy int64) ([]Invite, error) {
	rows, err := db.Query(
		`SELECT id, token, created_by, used_by, created_at, used_at
		 FROM invites WHERE created_by = ? ORDER BY created_at DESC`,
		createdBy,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var invites []Invite
	for rows.Next() {
		var inv Invite
		err := rows.Scan(&inv.ID, &inv.Token, &inv.CreatedBy, &inv.UsedBy, &inv.CreatedAt, &inv.UsedAt)
		if err != nil {
			return nil, err
		}
		invites = append(invites, inv)
	}
	return invites, rows.Err()
}

func DeleteInvite(db *sql.DB, id int64, createdBy int64) error {
	_, err := db.Exec("DELETE FROM invites WHERE id = ? AND created_by = ?", id, createdBy)
	return err
}
