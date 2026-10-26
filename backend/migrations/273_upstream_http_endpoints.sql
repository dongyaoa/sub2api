-- HTTP deployments and public IP:port origins use the same scoped site
-- authorization as HTTPS. Keep the scheme, non-default port and path prefix
-- in the identity so changing recipients never carries an old site's token.
CREATE OR REPLACE FUNCTION upstream_newapi_base(endpoint TEXT) RETURNS TEXT
LANGUAGE plpgsql IMMUTABLE AS $$
DECLARE parts TEXT[]; scheme TEXT; host TEXT; port TEXT; path TEXT; address INET;
BEGIN
    endpoint := btrim(endpoint);
    IF endpoint IS NULL OR strpos(endpoint, '%') > 0 OR strpos(endpoint, chr(92)) > 0 THEN RETURN ''; END IF;
    parts := regexp_match(endpoint, '^(https?)://(\[[0-9a-f:.]+\]|[^/:?#[:space:]@\[\]]+)(:([0-9]+))?(/[^?#[:space:]]*)?$', 'i');
    IF parts IS NULL THEN RETURN ''; END IF;
    scheme := lower(parts[1]);
    host := lower(parts[2]);
    port := COALESCE(parts[4], '');
    path := COALESCE(parts[5], '');
    IF left(host, 1)='[' THEN
        BEGIN
            address := substring(host FROM 2 FOR length(host)-2)::inet;
            IF family(address)<>6 THEN RETURN ''; END IF;
        EXCEPTION WHEN invalid_text_representation THEN
            RETURN '';
        END;
    END IF;
    IF port<>'' AND (port::numeric<1 OR port::numeric>65535) THEN RETURN ''; END IF;
    IF (scheme='http' AND port='80') OR (scheme='https' AND port='443') THEN port := ''; END IF;
    IF port<>'' THEN host := host || ':' || port; END IF;
    IF strpos(path, '//') > 0 OR path ~ '(^|/)[.]{1,2}(/|$)' THEN RETURN ''; END IF;
    path := regexp_replace(path, '/+$', '');
    path := regexp_replace(path, '/(v1/(chat/completions|responses|messages|models|usage)|v1beta/models|v1beta|v1)$', '');
    RETURN scheme || '://' || host || path;
END;
$$;
