//go:build windows

package service

import (
	"encoding/binary"
	"errors"
	"strings"

	"golang.org/x/sys/windows/registry"
)

const webAccessInternetSettingsKey = `Software\Microsoft\Windows\CurrentVersion\Internet Settings`

// The registry reader is private to the local-preview IP probe. It neither
// changes Windows settings nor affects API clients or other HTTP transports.
type webAccessRegistryKey interface {
	GetStringValue(string) (string, uint32, error)
	GetIntegerValue(string) (uint64, uint32, error)
	GetBinaryValue(string) ([]byte, uint32, error)
	Close() error
}

func readWebAccessSystemProxy() (webAccessSystemProxySettings, error) {
	return readWebAccessSystemProxyWith(func(path string) (webAccessRegistryKey, error) {
		return registry.OpenKey(registry.CURRENT_USER, path, registry.QUERY_VALUE)
	})
}

func readWebAccessSystemProxyWith(open func(string) (webAccessRegistryKey, error)) (webAccessSystemProxySettings, error) {
	settings := webAccessSystemProxySettings{}
	key, err := open(webAccessInternetSettingsKey)
	if errors.Is(err, registry.ErrNotExist) {
		return settings, nil
	}
	if err != nil {
		return settings, errors.New("cannot read Windows proxy settings")
	}
	defer func() { _ = key.Close() }()

	if settings.Enabled, err = readWebAccessRegistryBool(key, "ProxyEnable"); err != nil {
		return webAccessSystemProxySettings{}, err
	}
	if settings.Server, err = readWebAccessRegistryString(key, "ProxyServer"); err != nil {
		return webAccessSystemProxySettings{}, err
	}
	if settings.Override, err = readWebAccessRegistryString(key, "ProxyOverride"); err != nil {
		return webAccessSystemProxySettings{}, err
	}
	if settings.AutoConfigURL, err = readWebAccessRegistryString(key, "AutoConfigURL"); err != nil {
		return webAccessSystemProxySettings{}, err
	}
	if settings.AutoDetect, err = readWebAccessRegistryBool(key, "AutoDetect"); err != nil {
		return webAccessSystemProxySettings{}, err
	}

	flags, present, err := readWebAccessConnectionFlags(open)
	if err != nil {
		return webAccessSystemProxySettings{}, err
	}
	if present {
		// WinINet keeps old PAC URLs after automatic configuration is disabled.
		// The connection flags determine whether that URL is currently active.
		// Do not derive the manual proxy flag from this blob: clients commonly
		// update ProxyEnable/ProxyServer directly, leaving those blob fields stale.
		const autoURLFlag = uint32(0x04)
		const autoDetectFlag = uint32(0x08)
		settings.AutoDetect = flags&autoDetectFlag != 0
		if flags&autoURLFlag == 0 {
			settings.AutoConfigURL = ""
		} else if strings.TrimSpace(settings.AutoConfigURL) == "" {
			return webAccessSystemProxySettings{}, errors.New("automatic Windows proxy configuration is active but unavailable")
		}
	}
	return settings, nil
}

func readWebAccessRegistryBool(key webAccessRegistryKey, name string) (bool, error) {
	value, valueType, err := key.GetIntegerValue(name)
	if errors.Is(err, registry.ErrNotExist) {
		return false, nil
	}
	if err != nil || valueType != registry.DWORD || value > 1 {
		return false, errors.New("cannot read Windows proxy switch")
	}
	return value == 1, nil
}

func readWebAccessRegistryString(key webAccessRegistryKey, name string) (string, error) {
	value, valueType, err := key.GetStringValue(name)
	if errors.Is(err, registry.ErrNotExist) {
		return "", nil
	}
	if err != nil || (valueType != registry.SZ && valueType != registry.EXPAND_SZ) {
		return "", errors.New("cannot read Windows proxy configuration")
	}
	if valueType == registry.EXPAND_SZ {
		value, err = registry.ExpandString(value)
		if err != nil {
			return "", errors.New("cannot expand Windows proxy configuration")
		}
	}
	return value, nil
}

func readWebAccessConnectionFlags(open func(string) (webAccessRegistryKey, error)) (uint32, bool, error) {
	key, err := open(webAccessInternetSettingsKey + `\Connections`)
	if errors.Is(err, registry.ErrNotExist) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, errors.New("cannot read Windows automatic proxy settings")
	}
	defer func() { _ = key.Close() }()
	data, valueType, err := key.GetBinaryValue("DefaultConnectionSettings")
	if errors.Is(err, registry.ErrNotExist) {
		return 0, false, nil
	}
	if err != nil || valueType != registry.BINARY || len(data) < 12 {
		return 0, false, errors.New("cannot read Windows automatic proxy settings")
	}
	return binary.LittleEndian.Uint32(data[8:12]), true, nil
}
