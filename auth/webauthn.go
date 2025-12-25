package auth

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"math/big"
)

// WebAuthn constants
const (
	// COSE key types
	coseKtyEC2 = 2

	// COSE algorithms
	coseAlgES256 = -7 // ECDSA w/ SHA-256

	// COSE EC2 curves
	coseCrvP256 = 1

	// COSE key parameters
	coseKeyKty = 1
	coseKeyAlg = 3
	coseKeyCrv = -1
	coseKeyX   = -2
	coseKeyY   = -3

	// Authenticator data flags
	flagUserPresent  = 0x01
	flagUserVerified = 0x04
	flagAttestedCred = 0x40
	flagExtensions   = 0x80
)

var (
	errInvalidAuthData    = errors.New("webauthn: invalid authenticator data")
	errInvalidPublicKey   = errors.New("webauthn: invalid public key")
	errInvalidSignature   = errors.New("webauthn: invalid signature")
	errUserNotVerified    = errors.New("webauthn: user verification required")
	errInvalidAttestation = errors.New("webauthn: invalid attestation")
)

// Credential represents a stored WebAuthn credential
type Credential struct {
	ID        []byte
	PublicKey []byte // Uncompressed P-256 point (65 bytes: 0x04 || x || y)
	AAGUID    []byte
	SignCount uint32
}

// GenerateChallenge creates a random challenge for WebAuthn ceremonies
func GenerateChallenge() ([]byte, error) {
	b := make([]byte, 32)
	_, err := rand.Read(b)
	return b, err
}

// Base64URL encoding helpers
func base64URLEncode(data []byte) string {
	return base64.RawURLEncoding.EncodeToString(data)
}

func base64URLDecode(s string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(s)
}

// parseAuthenticatorData parses the authenticator data structure
// Returns: rpIdHash, flags, signCount, attestedCredData (if present), error
func parseAuthenticatorData(data []byte) (rpIdHash []byte, flags byte, signCount uint32, attestedCred []byte, err error) {
	if len(data) < 37 {
		return nil, 0, 0, nil, errInvalidAuthData
	}

	rpIdHash = data[0:32]
	flags = data[32]
	signCount = binary.BigEndian.Uint32(data[33:37])

	if flags&flagAttestedCred != 0 {
		if len(data) > 37 {
			attestedCred = data[37:]
		}
	}

	return rpIdHash, flags, signCount, attestedCred, nil
}

// parseAttestedCredentialData extracts credential ID and public key from attested credential data
func parseAttestedCredentialData(data []byte) (aaguid []byte, credentialID []byte, pubKey *ecdsa.PublicKey, err error) {
	if len(data) < 18 {
		return nil, nil, nil, errInvalidAuthData
	}

	aaguid = data[0:16]
	credIDLen := binary.BigEndian.Uint16(data[16:18])

	if len(data) < 18+int(credIDLen) {
		return nil, nil, nil, errInvalidAuthData
	}

	credentialID = data[18 : 18+credIDLen]
	coseKeyData := data[18+credIDLen:]

	pubKey, err = parseCOSEKey(coseKeyData)
	if err != nil {
		return nil, nil, nil, err
	}

	return aaguid, credentialID, pubKey, nil
}

// parseCOSEKey parses a COSE_Key structure and returns an ECDSA public key
func parseCOSEKey(data []byte) (*ecdsa.PublicKey, error) {
	decoded, err := cborDecode(data)
	if err != nil {
		return nil, err
	}

	m, ok := decoded.(map[any]any)
	if !ok {
		return nil, errInvalidPublicKey
	}

	// Check key type is EC2
	kty, ok := cborMapGetInt(m, coseKeyKty)
	if !ok || kty != coseKtyEC2 {
		return nil, errInvalidPublicKey
	}

	// Check algorithm is ES256
	alg, ok := cborMapGetInt(m, coseKeyAlg)
	if !ok || alg != coseAlgES256 {
		return nil, errInvalidPublicKey
	}

	// Check curve is P-256
	crv, ok := cborMapGetInt(m, coseKeyCrv)
	if !ok || crv != coseCrvP256 {
		return nil, errInvalidPublicKey
	}

	// Get x and y coordinates
	x, ok := cborMapGetBytes(m, coseKeyX)
	if !ok || len(x) != 32 {
		return nil, errInvalidPublicKey
	}

	y, ok := cborMapGetBytes(m, coseKeyY)
	if !ok || len(y) != 32 {
		return nil, errInvalidPublicKey
	}

	return &ecdsa.PublicKey{
		Curve: elliptic.P256(),
		X:     new(big.Int).SetBytes(x),
		Y:     new(big.Int).SetBytes(y),
	}, nil
}

// encodePublicKey encodes an ECDSA public key as uncompressed point (65 bytes)
func encodePublicKey(pub *ecdsa.PublicKey) []byte {
	b := make([]byte, 65)
	b[0] = 0x04
	pub.X.FillBytes(b[1:33])
	pub.Y.FillBytes(b[33:65])
	return b
}

// decodePublicKey decodes an uncompressed point to ECDSA public key
func decodePublicKey(data []byte) (*ecdsa.PublicKey, error) {
	if len(data) != 65 || data[0] != 0x04 {
		return nil, errInvalidPublicKey
	}
	return &ecdsa.PublicKey{
		Curve: elliptic.P256(),
		X:     new(big.Int).SetBytes(data[1:33]),
		Y:     new(big.Int).SetBytes(data[33:65]),
	}, nil
}

// ParseRegistrationResponse parses and validates a WebAuthn registration response
func ParseRegistrationResponse(
	attestationObject []byte,
	clientDataJSON []byte,
	expectedChallenge []byte,
	expectedOrigin string,
	expectedRPID string,
) (*Credential, error) {
	// Parse clientDataJSON
	clientData, err := parseClientData(clientDataJSON)
	if err != nil {
		return nil, err
	}

	// Verify clientData
	if clientData.Type != "webauthn.create" {
		return nil, errors.New("webauthn: invalid client data type")
	}

	challenge, err := base64URLDecode(clientData.Challenge)
	if err != nil || !bytesEqual(challenge, expectedChallenge) {
		return nil, errors.New("webauthn: challenge mismatch")
	}

	if clientData.Origin != expectedOrigin {
		return nil, errors.New("webauthn: origin mismatch")
	}

	// Parse attestationObject (CBOR)
	decoded, err := cborDecode(attestationObject)
	if err != nil {
		return nil, err
	}

	attObj, ok := decoded.(map[any]any)
	if !ok {
		return nil, errInvalidAttestation
	}

	// We accept "none" and "packed" attestation - we don't verify the attestation signature
	// since we're just trusting the authenticator

	authData, ok := cborMapGetBytesStr(attObj, "authData")
	if !ok {
		return nil, errInvalidAttestation
	}

	// Parse authenticator data
	rpIdHash, flags, signCount, attestedCred, err := parseAuthenticatorData(authData)
	if err != nil {
		return nil, err
	}

	// Verify RP ID hash
	expectedRPIDHash := sha256.Sum256([]byte(expectedRPID))
	if !bytesEqual(rpIdHash, expectedRPIDHash[:]) {
		return nil, errors.New("webauthn: RP ID mismatch")
	}

	// Verify user verification
	if flags&flagUserVerified == 0 {
		return nil, errUserNotVerified
	}

	// Parse attested credential data
	if attestedCred == nil {
		return nil, errInvalidAttestation
	}

	aaguid, credentialID, pubKey, err := parseAttestedCredentialData(attestedCred)
	if err != nil {
		return nil, err
	}

	return &Credential{
		ID:        credentialID,
		PublicKey: encodePublicKey(pubKey),
		AAGUID:    aaguid,
		SignCount: signCount,
	}, nil
}

// VerifyAssertionResponse verifies a WebAuthn authentication response
func VerifyAssertionResponse(
	credentialID []byte,
	authenticatorData []byte,
	clientDataJSON []byte,
	signature []byte,
	storedPublicKey []byte,
	storedSignCount uint32,
	expectedChallenge []byte,
	expectedOrigin string,
	expectedRPID string,
) (newSignCount uint32, err error) {
	// Parse clientDataJSON
	clientData, err := parseClientData(clientDataJSON)
	if err != nil {
		return 0, err
	}

	// Verify clientData
	if clientData.Type != "webauthn.get" {
		return 0, errors.New("webauthn: invalid client data type")
	}

	challenge, err := base64URLDecode(clientData.Challenge)
	if err != nil || !bytesEqual(challenge, expectedChallenge) {
		return 0, errors.New("webauthn: challenge mismatch")
	}

	if clientData.Origin != expectedOrigin {
		return 0, errors.New("webauthn: origin mismatch")
	}

	// Parse authenticator data
	rpIdHash, flags, signCount, _, err := parseAuthenticatorData(authenticatorData)
	if err != nil {
		return 0, err
	}

	// Verify RP ID hash
	expectedRPIDHash := sha256.Sum256([]byte(expectedRPID))
	if !bytesEqual(rpIdHash, expectedRPIDHash[:]) {
		return 0, errors.New("webauthn: RP ID mismatch")
	}

	// Verify user verification
	if flags&flagUserVerified == 0 {
		return 0, errUserNotVerified
	}

	// Verify sign count (should be greater than stored, or both zero)
	if storedSignCount > 0 && signCount <= storedSignCount {
		return 0, errors.New("webauthn: sign count not incremented (possible cloned authenticator)")
	}

	// Verify signature
	// The signature is over: authenticatorData || SHA-256(clientDataJSON)
	clientDataHash := sha256.Sum256(clientDataJSON)
	signedData := append(authenticatorData, clientDataHash[:]...)

	pubKey, err := decodePublicKey(storedPublicKey)
	if err != nil {
		return 0, err
	}

	if !verifyES256(pubKey, signedData, signature) {
		return 0, errInvalidSignature
	}

	return signCount, nil
}

// verifyES256 verifies an ECDSA P-256 signature in ASN.1 DER format
func verifyES256(pubKey *ecdsa.PublicKey, data, signature []byte) bool {
	hash := sha256.Sum256(data)

	// Parse ASN.1 DER signature
	r, s, err := parseASN1Signature(signature)
	if err != nil {
		return false
	}

	return ecdsa.Verify(pubKey, hash[:], r, s)
}

// parseASN1Signature parses an ASN.1 DER encoded ECDSA signature
func parseASN1Signature(sig []byte) (r, s *big.Int, err error) {
	if len(sig) < 8 {
		return nil, nil, errInvalidSignature
	}

	// SEQUENCE
	if sig[0] != 0x30 {
		return nil, nil, errInvalidSignature
	}

	// Length
	pos := 1
	seqLen := int(sig[pos])
	pos++

	if seqLen&0x80 != 0 {
		// Long form length (shouldn't happen for ECDSA sigs)
		return nil, nil, errInvalidSignature
	}

	// First INTEGER (r)
	if sig[pos] != 0x02 {
		return nil, nil, errInvalidSignature
	}
	pos++
	rLen := int(sig[pos])
	pos++
	r = new(big.Int).SetBytes(sig[pos : pos+rLen])
	pos += rLen

	// Second INTEGER (s)
	if sig[pos] != 0x02 {
		return nil, nil, errInvalidSignature
	}
	pos++
	sLen := int(sig[pos])
	pos++
	s = new(big.Int).SetBytes(sig[pos : pos+sLen])

	return r, s, nil
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
