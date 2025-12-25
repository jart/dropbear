package auth

import (
	"database/sql"
)

type User struct {
	ID          int64
	Name        string
	IsAdmin     bool
	Credentials []Credential
}

// Database operations

func CreateUser(db *sql.DB, name string, isAdmin bool) (*User, error) {
	result, err := db.Exec(
		"INSERT INTO users (name, is_admin) VALUES (?, ?)",
		name, isAdmin,
	)
	if err != nil {
		return nil, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}
	return &User{ID: id, Name: name, IsAdmin: isAdmin}, nil
}

func GetUserByID(db *sql.DB, id int64) (*User, error) {
	user := &User{}
	var isAdmin int
	err := db.QueryRow(
		"SELECT id, name, is_admin FROM users WHERE id = ?", id,
	).Scan(&user.ID, &user.Name, &isAdmin)
	if err != nil {
		return nil, err
	}
	user.IsAdmin = isAdmin != 0
	user.Credentials, err = GetCredentialsForUser(db, user.ID)
	if err != nil {
		return nil, err
	}
	return user, nil
}

func GetUserByName(db *sql.DB, name string) (*User, error) {
	user := &User{}
	var isAdmin int
	err := db.QueryRow(
		"SELECT id, name, is_admin FROM users WHERE name = ?", name,
	).Scan(&user.ID, &user.Name, &isAdmin)
	if err != nil {
		return nil, err
	}
	user.IsAdmin = isAdmin != 0
	user.Credentials, err = GetCredentialsForUser(db, user.ID)
	if err != nil {
		return nil, err
	}
	return user, nil
}

func CountUsers(db *sql.DB) (int, error) {
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM users").Scan(&count)
	return count, err
}

func SaveCredential(db *sql.DB, userID int64, cred *Credential) error {
	_, err := db.Exec(
		`INSERT INTO webauthn_credentials (user_id, credential_id, public_key, aaguid, sign_count)
		 VALUES (?, ?, ?, ?, ?)`,
		userID, cred.ID, cred.PublicKey, cred.AAGUID, cred.SignCount,
	)
	return err
}

func GetCredentialsForUser(db *sql.DB, userID int64) ([]Credential, error) {
	rows, err := db.Query(
		`SELECT credential_id, public_key, aaguid, sign_count
		 FROM webauthn_credentials WHERE user_id = ?`, userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var creds []Credential
	for rows.Next() {
		var cred Credential
		err := rows.Scan(&cred.ID, &cred.PublicKey, &cred.AAGUID, &cred.SignCount)
		if err != nil {
			return nil, err
		}
		creds = append(creds, cred)
	}
	return creds, rows.Err()
}

func UpdateCredentialSignCount(db *sql.DB, credentialID []byte, signCount uint32) error {
	_, err := db.Exec(
		"UPDATE webauthn_credentials SET sign_count = ? WHERE credential_id = ?",
		signCount, credentialID,
	)
	return err
}
