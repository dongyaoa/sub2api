package service

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/stretchr/testify/require"
)

const audioVoiceTestModel = "gemini-3.8-flash-tts"
const audioVoiceTestSupplierKey = "voicekey_private-profile-test"

func audioVoiceTestFixture(t *testing.T) (*OpenAIGatewayService, *APIKey, *Account, string, *AudioVoiceBinding) {
	t.Helper()
	cfg := &config.Config{}
	cfg.JWT.Secret = "voice-envelope-test-server-secret-32-characters"
	key := audioSpeechPricingTestKey(0.2)
	account := &Account{ID: 51, Platform: PlatformGemini, Type: AccountTypeAPIKey,
		Status: StatusActive, Schedulable: true, Concurrency: 2, GroupIDs: []int64{*key.GroupID},
		Credentials: map[string]any{"base_url": "https://voice.example/v1beta", "api_key": "supplier-test-credential",
			"model_mapping": map[string]any{audioVoiceTestModel: audioVoiceTestModel, "speech-alias": audioVoiceTestModel},
		},
	}
	svc := &OpenAIGatewayService{cfg: cfg, accountRepo: schedulerTestOpenAIAccountRepo{accounts: []Account{*account}}}
	token, err := svc.SealAudioVoice(key, account, audioVoiceTestModel, audioVoiceTestSupplierKey, time.Now().Add(time.Hour))
	require.NoError(t, err)
	binding, err := svc.OpenAudioVoice(key, token)
	require.NoError(t, err)
	return svc, key, account, token, binding
}

func TestAudioVoiceTokenRoundTripAndNoSupplierCredentialDisclosure(t *testing.T) {
	svc, key, account, token, binding := audioVoiceTestFixture(t)
	require.NoError(t, svc.CheckAudioVoiceService())
	require.True(t, strings.HasPrefix(token, AudioVoiceTokenPrefix))
	require.NotContains(t, token, audioVoiceTestSupplierKey)
	require.NotContains(t, token, "supplier-test-credential")
	require.Equal(t, account.ID, binding.AccountID)
	require.Equal(t, audioVoiceTestModel, binding.UpstreamModel)
	require.Equal(t, audioVoiceTestSupplierKey, binding.VoiceKey)
	encoded, err := json.Marshal(binding)
	require.NoError(t, err)
	require.JSONEq(t, `{}`, string(encoded), "binding is not a public API response")
	second, err := svc.SealAudioVoice(key, account, audioVoiceTestModel, audioVoiceTestSupplierKey, time.Now().Add(time.Hour))
	require.NoError(t, err)
	require.NotEqual(t, token, second, "each envelope gets an independent random nonce")
	ctx := WithAudioVoiceBinding(context.Background(), binding, "public", "routing")
	restored, ok := AudioVoiceBindingFromContext(ctx)
	require.True(t, ok)
	require.Same(t, binding, restored)
	public, routing := audioVoiceModelsFromContext(ctx, "fallback")
	require.Equal(t, "public", public)
	require.Equal(t, "routing", routing)
}

func TestAudioVoiceTokenRejectsTamperingAndServerSecretRotation(t *testing.T) {
	svc, key, _, token, _ := audioVoiceTestFixture(t)
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(token, AudioVoiceTokenPrefix))
	require.NoError(t, err)
	raw[len(raw)-1] ^= 1
	tampered := AudioVoiceTokenPrefix + base64.RawURLEncoding.EncodeToString(raw)
	for _, invalid := range []string{"", "voicekey_unmanaged", AudioVoiceTokenPrefix + "bad", token + "=", tampered,
		token[:len(token)-5], token[:20] + "\n" + token[20:], AudioVoiceTokenPrefix + strings.Repeat("A", audioVoiceMaxTokenBytes)} {
		binding, err := svc.OpenAudioVoice(key, invalid)
		require.ErrorIs(t, err, ErrAudioVoiceInvalid)
		require.Nil(t, binding)
	}
	svc.cfg.JWT.Secret = "a-different-server-secret-after-rotation"
	_, err = svc.OpenAudioVoice(key, token)
	require.ErrorIs(t, err, ErrAudioVoiceInvalid)
	svc.cfg.JWT.Secret = ""
	require.ErrorIs(t, svc.CheckAudioVoiceService(), ErrAudioVoiceServiceUnavailable)
	_, err = svc.OpenAudioVoice(key, token)
	require.ErrorIs(t, err, ErrAudioVoiceServiceUnavailable)
}

func TestAudioVoiceTokenSupportsBoundedStatelessProfile(t *testing.T) {
	svc, key, account, _, _ := audioVoiceTestFixture(t)
	largeKey := "voicekey_" + strings.Repeat("a", audioVoiceMaxUpstreamKeyBytes-len("voicekey_"))
	token, err := svc.SealAudioVoice(key, account, audioVoiceTestModel, largeKey, time.Now().Add(time.Hour))
	require.NoError(t, err)
	require.LessOrEqual(t, len(token), audioVoiceMaxTokenBytes)
	binding, err := svc.OpenAudioVoice(key, token)
	require.NoError(t, err)
	require.True(t, binding.VoiceKey == largeKey, "bounded provider profile must survive encryption without printing its contents")
	_, err = svc.SealAudioVoice(key, account, audioVoiceTestModel, largeKey+"a", time.Now().Add(time.Hour))
	require.ErrorIs(t, err, ErrAudioVoiceInvalid)
}

func TestAudioVoiceTokenRejectsCrossOwnerKeyAndGroup(t *testing.T) {
	svc, key, _, token, _ := audioVoiceTestFixture(t)
	for _, mutation := range []func(*APIKey){
		func(k *APIKey) { k.ID++ },
		func(k *APIKey) { k.UserID++; k.User.ID++ },
		func(k *APIKey) { *k.GroupID++; k.Group.ID++ },
		func(k *APIKey) { k.User.ID++ },
	} {
		other, user, group, groupID := *key, *key.User, *key.Group, *key.GroupID
		other.User, other.Group, other.GroupID = &user, &group, &groupID
		mutation(&other)
		_, err := svc.OpenAudioVoice(&other, token)
		require.ErrorIs(t, err, ErrAudioVoiceInvalid)
	}
}

func audioVoiceSealClaimsForTest(t *testing.T, svc *OpenAIGatewayService, claims map[string]any) string {
	t.Helper()
	aead, err := svc.audioVoiceAEAD()
	require.NoError(t, err)
	nonce := make([]byte, aead.NonceSize())
	_, err = rand.Read(nonce)
	require.NoError(t, err)
	plaintext, err := json.Marshal(claims)
	require.NoError(t, err)
	envelope := append([]byte{audioVoiceTokenVersion}, nonce...)
	envelope = aead.Seal(envelope, nonce, plaintext, []byte(audioVoiceTokenDomain))
	return AudioVoiceTokenPrefix + base64.RawURLEncoding.EncodeToString(envelope)
}

func TestAudioVoiceTokenExpiryAndStrictClaims(t *testing.T) {
	svc, key, account, _, _ := audioVoiceTestFixture(t)
	before := time.Now()
	for _, expiry := range []time.Time{{}, before.Add(30 * 24 * time.Hour)} {
		token, err := svc.SealAudioVoice(key, account, audioVoiceTestModel, audioVoiceTestSupplierKey, expiry)
		require.NoError(t, err)
		binding, err := svc.OpenAudioVoice(key, token)
		require.NoError(t, err)
		require.LessOrEqual(t, binding.ExpiresAt, time.Now().Add(AudioVoiceMaxTTL).Unix())
		require.GreaterOrEqual(t, binding.ExpiresAt, before.Add(AudioVoiceMaxTTL).Unix())
	}
	_, err := svc.SealAudioVoice(key, account, audioVoiceTestModel, audioVoiceTestSupplierKey, before.Add(-time.Hour))
	require.ErrorIs(t, err, ErrAudioVoiceInvalid)
	now := time.Now().Unix()
	for _, mutation := range []func(map[string]any){
		func(c map[string]any) { c["iat"] = now - 7200; c["exp"] = now - 1 },
		func(c map[string]any) { c["exp"] = now + int64(AudioVoiceMaxTTL/time.Second) + 1 },
		func(c map[string]any) { c["iat"] = now + 120 },
		func(c map[string]any) { c["unexpected"] = true },
		func(c map[string]any) { c["aid"] = 0 },
		func(c map[string]any) { c["v"] = 2 },
		func(c map[string]any) { c["voice"] = "voicekey_" },
	} {
		claims := map[string]any{"v": 1, "uid": key.UserID, "kid": key.ID, "gid": *key.GroupID, "aid": account.ID,
			"fp": audioVoiceCredentialFingerprint(account), "model": audioVoiceTestModel,
			"voice": audioVoiceTestSupplierKey, "iat": now, "exp": now + 3600}
		mutation(claims)
		_, err := svc.OpenAudioVoice(key, audioVoiceSealClaimsForTest(t, svc, claims))
		require.ErrorIs(t, err, ErrAudioVoiceInvalid)
	}
}

func TestAudioVoiceCloneBillingIDCannotCollapseUnderClientID(t *testing.T) {
	ctx := context.WithValue(context.Background(), ctxkey.ClientRequestID, "reused-client-id")
	require.Equal(t, "voice_clone:first", resolveUsageBillingRequestID(ctx, "voice_clone:first"))
	require.Equal(t, "voice_clone:second", resolveUsageBillingRequestID(ctx, "voice_clone:second"))
}
