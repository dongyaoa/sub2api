package service

import (
	"context"
	"errors"
	"strings"
)

func (s *UpstreamCenterService) applyMonitorSource(ctx context.Context, target *UpstreamTarget, in UpstreamTargetInput, key *string, recipientChanged bool) error {
	invalid := func(detail string) error {
		return ErrUpstreamInvalid.WithMetadata(map[string]string{"field": "source_target_id", "detail": detail})
	}
	if in.SourceTargetID == nil {
		// Editing credentials explicitly detaches the import. Cosmetic and schedule
		// edits keep the site's authorization reference and the copied inference key.
		if target.SupplierID != nil || in.SourceAccountID != nil || *key != "" || recipientChanged {
			target.SourceTargetID = nil
		}
		return nil
	}
	if *in.SourceTargetID < 0 {
		return invalid("select an existing supplier group, or zero to detach it")
	}
	if *in.SourceTargetID == 0 {
		target.SourceTargetID = nil
		return nil
	}
	if target.SupplierID != nil || len(target.AccountIDs) > 0 || in.SourceAccountID != nil || *key != "" {
		return invalid("use a source group only for an independent monitor without another credential source")
	}
	if target.ID > 0 && target.SourceTargetID != nil && *target.SourceTargetID == *in.SourceTargetID {
		if recipientChanged {
			return invalid("detach the source group before changing the endpoint or provider")
		}
		// Ordinary edits retain the copied key and monitor configuration even if
		// the source has since been archived; only authorization follows the source.
		return nil
	}
	source, err := s.repo.GetTarget(ctx, *in.SourceTargetID)
	if err != nil {
		return err
	}
	if source.SupplierID == nil || source.ID == target.ID {
		return invalid("select a supplier group rather than another independent monitor")
	}
	if _, err = s.repo.GetSupplier(ctx, *source.SupplierID); err != nil {
		return err
	}
	if in.Endpoint != nil && strings.TrimSpace(*in.Endpoint) != "" && normalizeEndpoint(*in.Endpoint) != normalizeEndpoint(source.Endpoint) ||
		in.Provider != nil && strings.TrimSpace(*in.Provider) != "" && strings.TrimSpace(*in.Provider) != source.Provider {
		return invalid("source group credentials must use that group's endpoint and provider")
	}
	*key, err = s.encryptor.Decrypt(source.APIKeyEncrypted)
	if err != nil {
		return ErrChannelMonitorAPIKeyDecryptFailed
	}
	id := source.ID
	target.SourceTargetID = &id
	target.Provider, target.Endpoint = source.Provider, normalizeEndpoint(source.Endpoint)
	if in.Name == nil || strings.TrimSpace(*in.Name) == "" {
		target.Name = source.Name
	}
	if in.APIMode == nil {
		target.APIMode = source.APIMode
	}
	if in.Models == nil {
		target.Models = append([]string{}, source.Models...)
	}
	return nil
}

func (s *UpstreamCenterService) inheritMonitorSourceAuthorization(ctx context.Context, target *UpstreamTarget) error {
	target.NewAPIUserID, target.NewAPIAccessTokenEncrypted, target.NewAPICredentialsInherited = 0, "", false
	source, err := s.repo.GetTarget(ctx, *target.SourceTargetID)
	if errors.Is(err, ErrUpstreamNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if source.SupplierID == nil || source.Provider != target.Provider || normalizeEndpoint(source.Endpoint) != normalizeEndpoint(target.Endpoint) {
		return nil
	}
	if _, err = s.repo.GetSupplier(ctx, *source.SupplierID); errors.Is(err, ErrUpstreamNotFound) {
		return nil
	} else if err != nil {
		return err
	}
	if source.NewAPIUserID > 0 && source.NewAPIAccessTokenEncrypted != "" {
		target.NewAPIUserID, target.NewAPIAccessTokenEncrypted = source.NewAPIUserID, source.NewAPIAccessTokenEncrypted
		target.NewAPICredentialsInherited = true
	}
	return nil
}
