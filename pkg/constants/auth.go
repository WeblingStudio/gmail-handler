package constants

const (
	// OAuth2GrantType is the grant type for JWT bearer tokens.
	OAuth2GrantType = "urn:ietf:params:oauth:grant-type:jwt-bearer"
	// OAuth2Assertion is the key for the assertion in the token request.
	OAuth2Assertion = "assertion"
	// OAuth2TokenURL is the URL for the Google OAuth2 token endpoint.
	OAuth2TokenURL = "https://oauth2.googleapis.com/token"

	// JWTClaimIssuer is the issuer claim.
	JWTClaimIssuer = "iss"
	// JWTClaimSubject is the subject claim.
	JWTClaimSubject = "sub"
	// JWTClaimScope is the scope claim.
	JWTClaimScope = "scope"
	// JWTClaimAudience is the audience claim.
	JWTClaimAudience = "aud"
	// JWTClaimExpiration is the expiration time claim.
	JWTClaimExpiration = "exp"
	// JWTClaimIssuedAt is the issued at claim.
	JWTClaimIssuedAt = "iat"

	// IAMServiceAccountPath is the format for the service account path.
	IAMServiceAccountPath = "projects/-/serviceAccounts/%s"

	// GrantType is the grant type for the token request.
	GrantType = "grant_type"

	// TokenResponseAccessToken is the key for the access token in the token response.
	TokenResponseAccessToken = "access_token"
	// TokenResponseExpiresIn is the key for the expiration time in the token response.
	TokenResponseExpiresIn = "expires_in"
	// TokenResponseTokenType is the key for the token type in the token response.
	TokenResponseTokenType = "token_type"
)
