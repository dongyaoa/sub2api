//go:build windows && unit

package service

import (
	"encoding/binary"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows/registry"
)

type webAccessRegistryFixture struct {
	strings  map[string]string
	integers map[string]uint64
	binary   []byte
	err      error
	closed   bool
}

func (key *webAccessRegistryFixture) GetStringValue(name string) (string, uint32, error) {
	if key.err != nil {
		return "", registry.SZ, key.err
	}
	value, found := key.strings[name]
	if !found {
		return "", 0, registry.ErrNotExist
	}
	return value, registry.SZ, nil
}

func (key *webAccessRegistryFixture) GetIntegerValue(name string) (uint64, uint32, error) {
	if key.err != nil {
		return 0, registry.DWORD, key.err
	}
	value, found := key.integers[name]
	if !found {
		return 0, 0, registry.ErrNotExist
	}
	return value, registry.DWORD, nil
}

func (key *webAccessRegistryFixture) GetBinaryValue(string) ([]byte, uint32, error) {
	if key.err != nil {
		return nil, registry.BINARY, key.err
	}
	if key.binary == nil {
		return nil, 0, registry.ErrNotExist
	}
	return key.binary, registry.BINARY, nil
}

func (key *webAccessRegistryFixture) Close() error {
	key.closed = true
	return nil
}

func webAccessRegistryFixtureOpener(base, connections *webAccessRegistryFixture) func(string) (webAccessRegistryKey, error) {
	return func(path string) (webAccessRegistryKey, error) {
		if path == webAccessInternetSettingsKey && base != nil {
			return base, nil
		}
		if path == webAccessInternetSettingsKey+`\Connections` && connections != nil {
			return connections, nil
		}
		return nil, registry.ErrNotExist
	}
}

func webAccessRegistryConnectionFixture(flags uint32) *webAccessRegistryFixture {
	data := make([]byte, 12)
	binary.LittleEndian.PutUint32(data[8:12], flags)
	return &webAccessRegistryFixture{binary: data}
}

func TestWebAccessWindowsProxyReadManualAndMissingValues(t *testing.T) {
	base := &webAccessRegistryFixture{
		integers: map[string]uint64{"ProxyEnable": 1},
		strings: map[string]string{
			"ProxyServer":   "proxy.example.test:8123",
			"ProxyOverride": "<local>;*.example.test",
		},
	}
	settings, err := readWebAccessSystemProxyWith(webAccessRegistryFixtureOpener(base, nil))
	require.NoError(t, err)
	require.True(t, settings.Enabled)
	require.Equal(t, "proxy.example.test:8123", settings.Server)
	require.Equal(t, "<local>;*.example.test", settings.Override)
	require.Empty(t, settings.AutoConfigURL)
	require.False(t, settings.AutoDetect)
	require.True(t, base.closed)

	settings, err = readWebAccessSystemProxyWith(webAccessRegistryFixtureOpener(nil, nil))
	require.NoError(t, err)
	require.Equal(t, webAccessSystemProxySettings{}, settings)

	settings, err = readWebAccessSystemProxyWith(webAccessRegistryFixtureOpener(&webAccessRegistryFixture{}, nil))
	require.NoError(t, err)
	require.Equal(t, webAccessSystemProxySettings{}, settings)
}

func TestWebAccessWindowsProxyConnectionFlagsIgnoreInactivePAC(t *testing.T) {
	base := &webAccessRegistryFixture{
		integers: map[string]uint64{"ProxyEnable": 1, "AutoDetect": 1},
		strings: map[string]string{
			"ProxyServer":   "proxy.example.test:8123",
			"AutoConfigURL": "https://config.example.test/old.pac",
		},
	}
	// Only DIRECT is set in the old connection blob. ProxyEnable remains the
	// authoritative manual switch; inactive automatic settings must be ignored.
	connections := webAccessRegistryConnectionFixture(0x01)
	settings, err := readWebAccessSystemProxyWith(webAccessRegistryFixtureOpener(base, connections))
	require.NoError(t, err)
	require.True(t, settings.Enabled)
	require.Equal(t, "proxy.example.test:8123", settings.Server)
	require.Empty(t, settings.AutoConfigURL)
	require.False(t, settings.AutoDetect)
	require.True(t, connections.closed)
}

func TestWebAccessWindowsProxyConnectionFlagsIdentifyActiveAutomaticSettings(t *testing.T) {
	base := &webAccessRegistryFixture{strings: map[string]string{"AutoConfigURL": "https://config.example.test/active.pac"}}
	settings, err := readWebAccessSystemProxyWith(webAccessRegistryFixtureOpener(base, webAccessRegistryConnectionFixture(0x0d)))
	require.NoError(t, err)
	require.NotEmpty(t, settings.AutoConfigURL)
	require.True(t, settings.AutoDetect)

	_, err = readWebAccessSystemProxyWith(webAccessRegistryFixtureOpener(&webAccessRegistryFixture{}, webAccessRegistryConnectionFixture(0x05)))
	require.EqualError(t, err, "automatic Windows proxy configuration is active but unavailable")
}

func TestWebAccessWindowsProxyReadErrorsNeverBecomeDirect(t *testing.T) {
	privateError := errors.New("access denied: proxy-user:proxy-password@proxy.example.test")
	for _, open := range []func(string) (webAccessRegistryKey, error){
		func(string) (webAccessRegistryKey, error) { return nil, privateError },
		webAccessRegistryFixtureOpener(&webAccessRegistryFixture{err: privateError}, nil),
		webAccessRegistryFixtureOpener(&webAccessRegistryFixture{}, &webAccessRegistryFixture{err: privateError}),
		func(path string) (webAccessRegistryKey, error) {
			if path == webAccessInternetSettingsKey {
				return &webAccessRegistryFixture{}, nil
			}
			return nil, privateError
		},
	} {
		settings, err := readWebAccessSystemProxyWith(open)
		require.Error(t, err)
		require.Equal(t, webAccessSystemProxySettings{}, settings)
		require.NotContains(t, err.Error(), "proxy-password")
		require.NotContains(t, err.Error(), "proxy.example.test")
	}
}

func TestWebAccessWindowsProxyRejectsMalformedSwitchOrConnectionData(t *testing.T) {
	_, err := readWebAccessSystemProxyWith(webAccessRegistryFixtureOpener(&webAccessRegistryFixture{integers: map[string]uint64{"ProxyEnable": 2}}, nil))
	require.Error(t, err)
	_, err = readWebAccessSystemProxyWith(webAccessRegistryFixtureOpener(&webAccessRegistryFixture{}, &webAccessRegistryFixture{binary: []byte{1, 2, 3}}))
	require.Error(t, err)
}
