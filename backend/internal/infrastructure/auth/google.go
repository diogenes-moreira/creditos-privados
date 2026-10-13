package auth

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/api/idtoken"
)

// GoogleIdentity is the verified subject of a Google ID token.
type GoogleIdentity struct {
	Email         string
	EmailVerified bool
	Name          string
	// HostedDomain is the Google Workspace domain, empty for consumer accounts.
	HostedDomain string
}

// GoogleVerifier checks Google ID tokens.
//
// Verification uses Google's own library rather than a hand-rolled JWKS check:
// signature validation, key rotation and issuer checks are exactly the places
// where a subtle mistake becomes an authentication bypass.
type GoogleVerifier struct {
	clientID string
	// hostedDomain, when set, requires the account to belong to that Workspace
	// domain. Without it any Google account can present a valid token, and only
	// the lookup against our own user table keeps it out.
	hostedDomain string
}

// NewGoogleVerifierOrNil builds a verifier, or returns nil when no client id is
// configured so sign-in with Google is simply unavailable.
func NewGoogleVerifierOrNil(clientID, hostedDomain string) *GoogleVerifier {
	clientID = strings.TrimSpace(clientID)
	if clientID == "" {
		return nil
	}
	return &GoogleVerifier{clientID: clientID, hostedDomain: strings.TrimSpace(hostedDomain)}
}

// Verify validates the token and returns its subject.
func (v *GoogleVerifier) Verify(ctx context.Context, rawToken string) (*GoogleIdentity, error) {
	// Validate checks the signature, the issuer and that the audience is this
	// client id, which is what stops a token minted for another application
	// from being replayed here.
	payload, err := idtoken.Validate(ctx, rawToken, v.clientID)
	if err != nil {
		return nil, fmt.Errorf("invalid Google token: %w", err)
	}

	email, _ := payload.Claims["email"].(string)
	if email == "" {
		return nil, fmt.Errorf("Google token carries no email")
	}
	verified, _ := payload.Claims["email_verified"].(bool)
	if !verified {
		// An unverified address proves nothing: anyone can claim it.
		return nil, fmt.Errorf("Google account email is not verified")
	}

	hd, _ := payload.Claims["hd"].(string)
	if v.hostedDomain != "" && !strings.EqualFold(hd, v.hostedDomain) {
		return nil, fmt.Errorf("account does not belong to %s", v.hostedDomain)
	}

	name, _ := payload.Claims["name"].(string)
	return &GoogleIdentity{
		Email:         strings.ToLower(strings.TrimSpace(email)),
		EmailVerified: verified,
		Name:          name,
		HostedDomain:  hd,
	}, nil
}
