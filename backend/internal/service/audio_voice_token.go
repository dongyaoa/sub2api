package service

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"
)

const (
	AudioVoiceTokenPrefix         = "s2voice_"
	AudioVoiceMaxTTL              = 7 * 24 * time.Hour
	audioVoiceMaxTokenBytes       = 512 * 1024
	audioVoiceMaxUpstreamKeyBytes = 256 * 1024
	audioVoiceTokenVersion        = 1
	audioVoiceTokenDomain         = "sub2api/audio-voice-token/v1"
)

var (
	ErrAudioVoiceInvalid            = errors.New("voice token is invalid, expired, or does not belong to this API key")
	ErrAudioVoiceUnavailable        = errors.New("the account bound to this voice is unavailable for this model")
	ErrAudioVoiceServiceUnavailable = errors.New("voice token service is unavailable")
)

// AudioVoiceBinding is server-only. Even accidental JSON serialization must
// not reveal the supplier's stateless key or its credential fingerprint.
type AudioVoiceBinding struct {
	AccountID             int64  `json:"-"`
	UpstreamModel         string `json:"-"`
	ExpiresAt             int64  `json:"-"`
	VoiceKey              string `json:"-"`
	ownerUserID           int64
	apiKeyID              int64
	groupID               int64
	issuedAt              int64
	credentialFingerprint string
}

type audioVoiceClaims struct {
	Version     int    `json:"v"`
	UserID      int64  `json:"uid"`
	APIKeyID    int64  `json:"kid"`
	GroupID     int64  `json:"gid"`
	AccountID   int64  `json:"aid"`
	Fingerprint string `json:"fp"`
	Model       string `json:"model"`
	VoiceKey    string `json:"voice"`
	IssuedAt    int64  `json:"iat"`
	ExpiresAt   int64  `json:"exp"`
}

type audioVoiceBindingContextKey struct{}
type audioVoiceRequestModelsContextKey struct{}
type audioVoiceRequestModels struct{ public, routing string }

// models optionally carries the public and pre-channel-routing model names.
// These stay separate from the post-channel model passed to account selection.
func WithAudioVoiceBinding(ctx context.Context, binding *AudioVoiceBinding, models ...string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if len(models) > 0 {
		requestModels := audioVoiceRequestModels{public: strings.TrimSpace(models[0])}
		if len(models) > 1 {
			requestModels.routing = strings.TrimSpace(models[1])
		}
		ctx = context.WithValue(ctx, audioVoiceRequestModelsContextKey{}, requestModels)
	}
	return context.WithValue(ctx, audioVoiceBindingContextKey{}, binding)
}

func AudioVoiceBindingFromContext(ctx context.Context) (*AudioVoiceBinding, bool) {
	if ctx == nil {
		return nil, false
	}
	binding, ok := ctx.Value(audioVoiceBindingContextKey{}).(*AudioVoiceBinding)
	return binding, ok && binding != nil
}

// CheckAudioVoiceService is safe to call before a paid upstream creation.
func (s *OpenAIGatewayService) CheckAudioVoiceService() error {
	_, err := s.audioVoiceAEAD()
	return err
}

func (s *OpenAIGatewayService) audioVoiceAEAD() (cipher.AEAD, error) {
	if s == nil || s.cfg == nil || strings.TrimSpace(s.cfg.JWT.Secret) == "" {
		return nil, ErrAudioVoiceServiceUnavailable
	}
	// A separate purpose-derived key keeps voice envelopes independent from
	// JWT signatures and other uses of the server secret.
	derive := hmac.New(sha256.New, []byte(s.cfg.JWT.Secret))
	_, _ = derive.Write([]byte(audioVoiceTokenDomain + "/aes-256-gcm"))
	block, err := aes.NewCipher(derive.Sum(nil))
	if err != nil {
		return nil, ErrAudioVoiceServiceUnavailable
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, ErrAudioVoiceServiceUnavailable
	}
	return aead, nil
}

func audioVoiceOwner(apiKey *APIKey) (userID, keyID, groupID int64, ok bool) {
	if apiKey == nil || apiKey.ID <= 0 || apiKey.GroupID == nil || *apiKey.GroupID <= 0 || apiKey.Group == nil || apiKey.Group.ID != *apiKey.GroupID {
		return 0, 0, 0, false
	}
	userID = apiKey.UserID
	if apiKey.User != nil {
		if userID > 0 && apiKey.User.ID != userID {
			return 0, 0, 0, false
		}
		userID = apiKey.User.ID
	}
	return userID, apiKey.ID, *apiKey.GroupID, userID > 0
}

func audioVoiceCredentialFingerprint(account *Account) string {
	if account == nil {
		return ""
	}
	base := strings.TrimSpace(account.GetCredential("base_url"))
	if account.Platform == PlatformOpenAI {
		base = account.GetOpenAIFormatBaseURL()
		if base == "" {
			base = "https://api.openai.com"
		}
	}
	// JSON provides unambiguous field boundaries, including URLs with custom
	// prefixes. Only the digest is sealed; the raw credential never is.
	data, _ := json.Marshal([]string{account.Platform, base, strings.TrimSpace(account.GetCredential("api_key"))})
	sum := sha256.Sum256(data)
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func (s *OpenAIGatewayService) SealAudioVoice(apiKey *APIKey, account *Account, upstreamModel, voiceKey string, expiresAt time.Time) (string, error) {
	aead, err := s.audioVoiceAEAD()
	if err != nil {
		return "", err
	}
	userID, keyID, groupID, ok := audioVoiceOwner(apiKey)
	upstreamModel, voiceKey = strings.TrimSpace(upstreamModel), strings.TrimSpace(voiceKey)
	if !ok || account == nil || account.ID <= 0 || !SupportsAudioSpeechAccount(account) ||
		!openAIStickyAccountMatchesGroup(account, &groupID) ||
		upstreamModel == "" || len(upstreamModel) > 512 || !strings.HasPrefix(voiceKey, "voicekey_") || len(voiceKey) <= len("voicekey_") || len(voiceKey) > audioVoiceMaxUpstreamKeyBytes {
		return "", ErrAudioVoiceInvalid
	}
	now := time.Now().UTC()
	limit := now.Add(AudioVoiceMaxTTL)
	if expiresAt.IsZero() || expiresAt.After(limit) {
		expiresAt = limit
	}
	if expiresAt.Unix() <= now.Unix() {
		return "", ErrAudioVoiceInvalid
	}
	claims := audioVoiceClaims{
		Version: audioVoiceTokenVersion, UserID: userID, APIKeyID: keyID, GroupID: groupID,
		AccountID: account.ID, Fingerprint: audioVoiceCredentialFingerprint(account),
		Model: upstreamModel, VoiceKey: voiceKey, IssuedAt: now.Unix(), ExpiresAt: expiresAt.Unix(),
	}
	plaintext, err := json.Marshal(claims)
	if err != nil {
		return "", ErrAudioVoiceServiceUnavailable
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", ErrAudioVoiceServiceUnavailable
	}
	envelope := append([]byte{audioVoiceTokenVersion}, nonce...)
	envelope = aead.Seal(envelope, nonce, plaintext, []byte(audioVoiceTokenDomain))
	token := AudioVoiceTokenPrefix + base64.RawURLEncoding.EncodeToString(envelope)
	if len(token) > audioVoiceMaxTokenBytes {
		return "", ErrAudioVoiceInvalid
	}
	return token, nil
}

func (s *OpenAIGatewayService) OpenAudioVoice(apiKey *APIKey, token string) (*AudioVoiceBinding, error) {
	aead, err := s.audioVoiceAEAD()
	if err != nil {
		return nil, err
	}
	if len(token) > audioVoiceMaxTokenBytes || !strings.HasPrefix(token, AudioVoiceTokenPrefix) {
		return nil, ErrAudioVoiceInvalid
	}
	encoded := strings.TrimPrefix(token, AudioVoiceTokenPrefix)
	envelope, err := base64.RawURLEncoding.Strict().DecodeString(encoded)
	if err != nil || base64.RawURLEncoding.EncodeToString(envelope) != encoded || len(envelope) < 1+aead.NonceSize()+aead.Overhead() || envelope[0] != audioVoiceTokenVersion {
		return nil, ErrAudioVoiceInvalid
	}
	nonce, ciphertext := envelope[1:1+aead.NonceSize()], envelope[1+aead.NonceSize():]
	plaintext, err := aead.Open(nil, nonce, ciphertext, []byte(audioVoiceTokenDomain))
	if err != nil {
		return nil, ErrAudioVoiceInvalid
	}
	var claims audioVoiceClaims
	decoder := json.NewDecoder(bytes.NewReader(plaintext))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&claims); err != nil {
		return nil, ErrAudioVoiceInvalid
	}
	if err := decoder.Decode(new(any)); err != io.EOF || claims.Version != audioVoiceTokenVersion {
		return nil, ErrAudioVoiceInvalid
	}
	binding := &AudioVoiceBinding{
		AccountID: claims.AccountID, UpstreamModel: claims.Model, ExpiresAt: claims.ExpiresAt, VoiceKey: claims.VoiceKey,
		ownerUserID: claims.UserID, apiKeyID: claims.APIKeyID, groupID: claims.GroupID,
		issuedAt: claims.IssuedAt, credentialFingerprint: claims.Fingerprint,
	}
	if !binding.validFor(apiKey, time.Now()) {
		return nil, ErrAudioVoiceInvalid
	}
	return binding, nil
}

func (b *AudioVoiceBinding) validFor(apiKey *APIKey, now time.Time) bool {
	userID, keyID, groupID, ok := audioVoiceOwner(apiKey)
	return ok && b != nil && b.ownerUserID == userID && b.apiKeyID == keyID && b.groupID == groupID &&
		b.AccountID > 0 && b.UpstreamModel != "" && len(b.UpstreamModel) <= 512 &&
		strings.HasPrefix(b.VoiceKey, "voicekey_") && len(b.VoiceKey) > len("voicekey_") && len(b.VoiceKey) <= audioVoiceMaxUpstreamKeyBytes &&
		len(b.credentialFingerprint) == base64.RawURLEncoding.EncodedLen(sha256.Size) &&
		b.issuedAt > 0 && b.issuedAt <= now.Unix()+30 && b.ExpiresAt > now.Unix() &&
		b.ExpiresAt > b.issuedAt && b.ExpiresAt-b.issuedAt <= int64(AudioVoiceMaxTTL/time.Second)
}
