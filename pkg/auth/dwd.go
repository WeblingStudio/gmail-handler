package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/vinm0/gmail-handler/pkg/constants"
	"golang.org/x/oauth2"
	"google.golang.org/api/iamcredentials/v1"
)

// KeylessTokenSource implements oauth2.TokenSource.
// It uses the GCP IAM Credentials API to sign a JWT for Domain-Wide Delegation,
// avoiding the need for a local private key file.
type KeylessTokenSource struct {
	// ServiceAccountEmail is the email of the "Robot" (Cloud Function Identity)
	ServiceAccountEmail string

	// DelegateEmail is the Workspace User to impersonate (e.g. admin@ or notifications@)
	DelegateEmail string

	// Scopes are the permissions requested (GmailSend, GmailModify)
	Scopes []string

	// IamClient is the authenticated client capable of calling the IAM Credentials API
	IamClient *iamcredentials.Service
}

// Token satisfies the oauth2.TokenSource interface.
// It handles the generation of the signed JWT and the exchange for an Access Token.
func (k *KeylessTokenSource) Token() (*oauth2.Token, error) {
	ctx := context.Background()

	// 1. Construct the JWT Claim Set
	// This mirrors the standard Google Service Account JWT format
	iat := time.Now().Unix()
	exp := iat + 3600 // Token valid for 1 hour

	claims := map[string]interface{}{
		constants.JWTClaimIssuer:    k.ServiceAccountEmail,
		constants.JWTClaimSubject:   k.DelegateEmail, // The user we are impersonating
		constants.JWTClaimScope:     k.Scopes,        // Permissions
		constants.JWTClaimAudience:  constants.OAuth2TokenURL,
		constants.JWTClaimExpiration: exp,
		constants.JWTClaimIssuedAt:   iat,
	}

	payload, err := json.Marshal(claims)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal claims: %v", err)
	}

	// 2. Request the IAM Credentials API to sign these bytes
	// We use "SignJwt" which handles the cryptographic signature using Google-managed keys
	name := fmt.Sprintf(constants.IAMServiceAccountPath, k.ServiceAccountEmail)
	signReq := &iamcredentials.SignJwtRequest{
		Payload: string(payload),
	}

	resp, err := k.IamClient.Projects.ServiceAccounts.SignJwt(name, signReq).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("failed to sign jwt via IAM API: %v", err)
	}

	// 3. Exchange the Signed JWT for an OAuth Access Token
	// We perform a manual POST to the Google OAuth2 token endpoint
	v := url.Values{}
	v.Set(constants.GrantType, constants.OAuth2GrantType)
	v.Set(constants.OAuth2Assertion, resp.SignedJwt)

	tokenResp, err := http.PostForm(constants.OAuth2TokenURL, v)
	if err != nil {
		return nil, fmt.Errorf("failed to exchange jwt for token: %v", err)
	}
	defer tokenResp.Body.Close()

	if tokenResp.StatusCode != http.StatusOK {
		// Read body for error details
		return nil, fmt.Errorf("oauth endpoint returned status: %s", tokenResp.Status)
	}

	// 4. Parse the successful response
	var result struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
		TokenType   string `json:"token_type"`
	}
	if err := json.NewDecoder(tokenResp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode token response: %v", err)
	}

	// Return a standard oauth2.Token
	return &oauth2.Token{
		AccessToken: result.AccessToken,
		TokenType:   result.TokenType,
		Expiry:      time.Now().Add(time.Duration(result.ExpiresIn) * time.Second),
	}, nil
}
