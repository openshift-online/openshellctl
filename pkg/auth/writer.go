package auth

// WriteBundle persists a token as an oidc_token.json bundle in the Rust CLI
// schema (spec §5.2 writer.go). It is used by `token refresh --write` /
// --write-token to hand a freshly-obtained token (e.g. from a client-credentials
// exchange, which otherwise stays in memory) to the on-disk store the real
// openshell CLI reads. No refresh_token is written (client-credentials grants
// have none). The Writer is expected to write 0600 atomically.
func WriteBundle(w Writer, tok *Token) error {
	b := DiskBundle{
		AccessToken: tok.AccessToken,
		Issuer:      tok.Issuer,
		ClientID:    tok.ClientID,
	}
	if !tok.Expiry.IsZero() {
		exp := tok.Expiry.Unix()
		b.ExpiresAt = &exp
	}
	data, err := b.Marshal()
	if err != nil {
		return err
	}
	return w.WriteFile("oidc_token.json", data, 0o600)
}
