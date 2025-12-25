package auth

import (
	"database/sql"
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"sync"
	"time"
)

type Auth struct {
	db         *sql.DB
	rpID       string
	origin     string
	challenges sync.Map // in-memory challenge storage (keyed by name or name:invite)
}

type challengeData struct {
	challenge []byte
	name      string
	isAdmin   bool
	invite    string
	expiresAt time.Time
}

var (
	Unsecure = flag.Bool("unsecure", false, "Disable authentication (for local testing)")
)

func New(db *sql.DB, rpID, origin string) (*Auth, error) {
	return &Auth{db: db, rpID: rpID, origin: origin}, nil
}

func (a *Auth) DB() *sql.DB {
	return a.db
}

// Middleware returns the authenticated user or nil
func (a *Auth) GetUser(r *http.Request) *User {
	token := GetSessionToken(r)
	if token == "" {
		return nil
	}
	user, err := GetUserFromSession(a.db, token)
	if err != nil {
		return nil
	}
	return user
}

// RequireAuth middleware redirects to login if not authenticated
func (a *Auth) RequireAuth(next http.HandlerFunc) http.HandlerFunc {
	if *Unsecure {
		return next
	}
	return func(w http.ResponseWriter, r *http.Request) {
		if a.GetUser(r) == nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		next(w, r)
	}
}

// RequireAdmin middleware requires admin user
func (a *Auth) RequireAdmin(next http.HandlerFunc) http.HandlerFunc {
	if *Unsecure {
		return next
	}
	return func(w http.ResponseWriter, r *http.Request) {
		user := a.GetUser(r)
		if user == nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		if !user.IsAdmin {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		next(w, r)
	}
}

// Registration handlers

func (a *Auth) HandleRegisterPage(w http.ResponseWriter, r *http.Request) {
	inviteToken := r.URL.Query().Get("token")

	// Check if this is first user (no invite needed)
	count, err := CountUsers(a.db)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	isFirstUser := count == 0

	if !isFirstUser {
		// Validate invite
		if inviteToken == "" {
			http.Error(w, "Invite required", http.StatusForbidden)
			return
		}
		inv, err := GetInvite(a.db, inviteToken)
		if err != nil || inv.UsedBy != nil {
			http.Error(w, "Invalid or used invite", http.StatusForbidden)
			return
		}
	}

	serveFile("register.html")(w, r)
}

func (a *Auth) HandleRegisterBegin(w http.ResponseWriter, r *http.Request) {
	inviteToken := r.URL.Query().Get("token")

	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		http.Error(w, "Name required", http.StatusBadRequest)
		return
	}

	// Check if first user
	count, err := CountUsers(a.db)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	isFirstUser := count == 0
	isAdmin := isFirstUser

	if !isFirstUser {
		if inviteToken == "" {
			http.Error(w, "Invite required", http.StatusForbidden)
			return
		}
		inv, err := GetInvite(a.db, inviteToken)
		if err != nil || inv.UsedBy != nil {
			http.Error(w, "Invalid or used invite", http.StatusForbidden)
			return
		}
	}

	// Check name not taken
	if _, err := GetUserByName(a.db, req.Name); err == nil {
		http.Error(w, "Name already taken", http.StatusConflict)
		return
	}

	// Generate challenge
	challenge, err := GenerateChallenge()
	if err != nil {
		http.Error(w, "Failed to generate challenge", http.StatusInternalServerError)
		return
	}

	// Store challenge
	challengeKey := req.Name + ":" + inviteToken
	a.challenges.Store(challengeKey, &challengeData{
		challenge: challenge,
		name:      req.Name,
		isAdmin:   isAdmin,
		invite:    inviteToken,
		expiresAt: time.Now().Add(5 * time.Minute),
	})

	// Build WebAuthn options
	// Note: userID is just a placeholder - we'll assign real ID after successful registration
	userID := []byte(req.Name)

	options := map[string]any{
		"publicKey": map[string]any{
			"challenge": base64URLEncode(challenge),
			"rp": map[string]string{
				"name": "Dropbear",
				"id":   a.rpID,
			},
			"user": map[string]any{
				"id":          base64URLEncode(userID),
				"name":        req.Name,
				"displayName": req.Name,
			},
			"pubKeyCredParams": []map[string]any{
				{"type": "public-key", "alg": coseAlgES256},
			},
			"authenticatorSelection": map[string]any{
				"authenticatorAttachment": "cross-platform",
				"userVerification":        "required",
			},
			"timeout":     5 * 60 * 1000,
			"attestation": "none",
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(options)
}

func (a *Auth) HandleRegisterFinish(w http.ResponseWriter, r *http.Request) {
	inviteToken := r.URL.Query().Get("token")

	var req struct {
		Name     string `json:"name"`
		Response struct {
			ID       string `json:"id"`
			RawID    string `json:"rawId"`
			Type     string `json:"type"`
			Response struct {
				ClientDataJSON    string `json:"clientDataJSON"`
				AttestationObject string `json:"attestationObject"`
			} `json:"response"`
		} `json:"response"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	challengeKey := req.Name + ":" + inviteToken
	data, ok := a.challenges.Load(challengeKey)
	if !ok {
		http.Error(w, "No pending registration", http.StatusBadRequest)
		return
	}
	cd := data.(*challengeData)
	a.challenges.Delete(challengeKey)

	if time.Now().After(cd.expiresAt) {
		http.Error(w, "Registration expired", http.StatusBadRequest)
		return
	}

	// Decode base64url data
	clientDataJSON, err := base64URLDecode(req.Response.Response.ClientDataJSON)
	if err != nil {
		http.Error(w, "Invalid clientDataJSON", http.StatusBadRequest)
		return
	}

	attestationObject, err := base64URLDecode(req.Response.Response.AttestationObject)
	if err != nil {
		http.Error(w, "Invalid attestationObject", http.StatusBadRequest)
		return
	}

	// Parse and verify registration
	cred, err := ParseRegistrationResponse(
		attestationObject,
		clientDataJSON,
		cd.challenge,
		a.origin,
		a.rpID,
	)
	if err != nil {
		log.Printf("Registration verification error: %v", err)
		http.Error(w, "Registration verification failed: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Create user in database
	user, err := CreateUser(a.db, cd.name, cd.isAdmin)
	if err != nil {
		log.Printf("CreateUser error: %v", err)
		http.Error(w, "Failed to create user", http.StatusInternalServerError)
		return
	}

	// Save credential
	if err := SaveCredential(a.db, user.ID, cred); err != nil {
		log.Printf("SaveCredential error: %v", err)
		http.Error(w, "Failed to save credential", http.StatusInternalServerError)
		return
	}

	// Mark invite as used
	if cd.invite != "" {
		UseInvite(a.db, cd.invite, user.ID)
	}

	// Create session
	token, err := CreateSession(a.db, user.ID)
	if err != nil {
		log.Printf("CreateSession error: %v", err)
		http.Error(w, "Failed to create session", http.StatusInternalServerError)
		return
	}

	SetSessionCookie(w, token)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// Login handlers

func (a *Auth) HandleLoginPage(w http.ResponseWriter, r *http.Request) {
	serveFile("login.html")(w, r)
}

func (a *Auth) HandleLoginBegin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		http.Error(w, "Name required", http.StatusBadRequest)
		return
	}

	user, err := GetUserByName(a.db, req.Name)
	if err != nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	if len(user.Credentials) == 0 {
		http.Error(w, "No credentials registered", http.StatusBadRequest)
		return
	}

	// Generate challenge
	challenge, err := GenerateChallenge()
	if err != nil {
		http.Error(w, "Failed to generate challenge", http.StatusInternalServerError)
		return
	}

	a.challenges.Store("login:"+req.Name, &challengeData{
		challenge: challenge,
		name:      req.Name,
		expiresAt: time.Now().Add(5 * time.Minute),
	})

	// Build allowCredentials list
	allowCreds := make([]map[string]any, len(user.Credentials))
	for i, cred := range user.Credentials {
		allowCreds[i] = map[string]any{
			"type": "public-key",
			"id":   base64URLEncode(cred.ID),
		}
	}

	options := map[string]any{
		"publicKey": map[string]any{
			"challenge":        base64URLEncode(challenge),
			"rpId":             a.rpID,
			"allowCredentials": allowCreds,
			"userVerification": "required",
			"timeout":          5 * 60 * 1000,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(options)
}

func (a *Auth) HandleLoginFinish(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name     string `json:"name"`
		Response struct {
			ID       string `json:"id"`
			RawID    string `json:"rawId"`
			Type     string `json:"type"`
			Response struct {
				ClientDataJSON    string `json:"clientDataJSON"`
				AuthenticatorData string `json:"authenticatorData"`
				Signature         string `json:"signature"`
				UserHandle        string `json:"userHandle"`
			} `json:"response"`
		} `json:"response"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	data, ok := a.challenges.Load("login:" + req.Name)
	if !ok {
		http.Error(w, "No pending login", http.StatusBadRequest)
		return
	}
	cd := data.(*challengeData)
	a.challenges.Delete("login:" + req.Name)

	if time.Now().After(cd.expiresAt) {
		http.Error(w, "Login expired", http.StatusBadRequest)
		return
	}

	user, err := GetUserByName(a.db, req.Name)
	if err != nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	// Decode base64url data
	credentialID, err := base64URLDecode(req.Response.RawID)
	if err != nil {
		http.Error(w, "Invalid credential ID", http.StatusBadRequest)
		return
	}

	clientDataJSON, err := base64URLDecode(req.Response.Response.ClientDataJSON)
	if err != nil {
		http.Error(w, "Invalid clientDataJSON", http.StatusBadRequest)
		return
	}

	authenticatorData, err := base64URLDecode(req.Response.Response.AuthenticatorData)
	if err != nil {
		http.Error(w, "Invalid authenticatorData", http.StatusBadRequest)
		return
	}

	signature, err := base64URLDecode(req.Response.Response.Signature)
	if err != nil {
		http.Error(w, "Invalid signature", http.StatusBadRequest)
		return
	}

	// Find the matching credential
	var matchedCred *Credential
	for i := range user.Credentials {
		if bytesEqual(user.Credentials[i].ID, credentialID) {
			matchedCred = &user.Credentials[i]
			break
		}
	}
	if matchedCred == nil {
		http.Error(w, "Unknown credential", http.StatusBadRequest)
		return
	}

	// Verify assertion
	newSignCount, err := VerifyAssertionResponse(
		credentialID,
		authenticatorData,
		clientDataJSON,
		signature,
		matchedCred.PublicKey,
		matchedCred.SignCount,
		cd.challenge,
		a.origin,
		a.rpID,
	)
	if err != nil {
		log.Printf("Login verification error: %v", err)
		http.Error(w, "Authentication failed: "+err.Error(), http.StatusUnauthorized)
		return
	}

	// Update sign count
	UpdateCredentialSignCount(a.db, credentialID, newSignCount)

	// Create session
	token, err := CreateSession(a.db, user.ID)
	if err != nil {
		http.Error(w, "Failed to create session", http.StatusInternalServerError)
		return
	}

	SetSessionCookie(w, token)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (a *Auth) HandleLogout(w http.ResponseWriter, r *http.Request) {
	token := GetSessionToken(r)
	if token != "" {
		DeleteSession(a.db, token)
	}
	ClearSessionCookie(w)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

// Invite handlers

func (a *Auth) HandleInvitesPage(w http.ResponseWriter, r *http.Request) {
	user := a.GetUser(r)
	if user == nil || !user.IsAdmin {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	serveFile("invites.html")(w, r)
}

func (a *Auth) HandleCreateInvite(w http.ResponseWriter, r *http.Request) {
	user := a.GetUser(r)
	if user == nil || !user.IsAdmin {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	token, err := CreateInvite(a.db, user.ID)
	if err != nil {
		http.Error(w, "Failed to create invite", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"token": token})
}

func (a *Auth) HandleListInvites(w http.ResponseWriter, r *http.Request) {
	user := a.GetUser(r)
	if user == nil || !user.IsAdmin {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	invites, err := ListInvites(a.db, user.ID)
	if err != nil {
		http.Error(w, "Failed to list invites", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(invites)
}
