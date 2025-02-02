package provider

import (
	"context"
	"errors"
	"github.com/thomseddon/traefik-forward-auth/internal/pkce"
	"strings"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

// OIDC provider
type OIDC struct {
	IssuerURL    string `long:"issuer-url" env:"ISSUER_URL" description:"Issuer URL"`
	ClientID     string `long:"client-id" env:"CLIENT_ID" description:"Client ID"`
	ClientSecret string `long:"client-secret" env:"CLIENT_SECRET" description:"Client Secret" json:"-"`
	PkceRequired bool   `long:"pkce-required" env:"PKCE_REQUIRED" description:"Optional pkce required indicator"`
	WebId        bool 	`long:"webid" env:"WEBID" description:"require scope webid"`

	OAuthProvider

	provider     *oidc.Provider
	verifier     *oidc.IDTokenVerifier
	pkceVerifier *pkce.CodeVerifier
}

// Name returns the name of the provider
func (o *OIDC) Name() string {
	return "oidc"
}

// Setup performs validation and setup
func (o *OIDC) Setup() error {
	// Check params
	if err := o.checkParams(); err != nil {
		return err
	}

	var err error
	o.ctx = context.Background()

	// Try to initiate provider
	o.provider, err = oidc.NewProvider(o.ctx, o.IssuerURL)
	if err != nil {
		return err
	}

	scopes:= []string{oidc.ScopeOpenID, "profile", "email"};
	if(o.WebId) {
		scopes = []string{oidc.ScopeOpenID, "profile", "webid"};
	}
	// Create oauth2 config
	o.Config = &oauth2.Config{
		ClientID:     o.ClientID,
		ClientSecret: o.ClientSecret,
		Endpoint:     o.provider.Endpoint(),

		// "openid" is a required scope for OpenID Connect flows.
		Scopes: scopes,
	}

	// Create OIDC verifier
	o.verifier = o.provider.Verifier(&oidc.Config{
		ClientID: o.ClientID,
	})

	return nil
}

// GetLoginURL provides the login url for the given redirect uri and state
func (o *OIDC) GetLoginURL(redirectURI, state string) string {
	var opts []oauth2.AuthCodeOption
	if o.PkceRequired {
		v, e := pkce.CreateCodeVerifier()
		if e != nil {
			panic(e)
		}
		o.pkceVerifier = v
		opts = append(opts, oauth2.SetAuthURLParam("code_challenge_method", "S256"))
		opts = append(opts, oauth2.SetAuthURLParam("code_challenge", o.pkceVerifier.CodeChallengeS256()))
	}
	return o.OAuthGetLoginURL(redirectURI, state, opts...)
}

// ExchangeCode exchanges the given redirect uri and code for a token
func (o *OIDC) ExchangeCode(redirectURI, code string) (string, error) {
	var opts []oauth2.AuthCodeOption

	if o.PkceRequired {
		opts = append(opts, oauth2.SetAuthURLParam("code_verifier", o.pkceVerifier.String()))
	}

	token, err := o.OAuthExchangeCode(redirectURI, code, opts...)
	if err != nil {
		return "", err
	}

	// Extract ID token
	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		return "", errors.New("Missing id_token")
	}

	return rawIDToken, nil
}

// GetUser uses the given token and returns a complete provider.User object
func (o *OIDC) GetUser(token string) (User, error) {
	var user User

	// Parse & Verify ID Token
	idToken, err := o.verifier.Verify(o.ctx, token)
	if err != nil {
		return user, err
	}

	// Extract custom claims
	if err := idToken.Claims(&user); err != nil {
		return user, err
	}

	return user, nil
}

func (o *OIDC) checkParams() error {
	if o.IssuerURL == "" || o.ClientID == "" || (o.ClientSecret == "" && !o.PkceRequired) {
		var emptyFields []string

		if o.IssuerURL == "" {
			emptyFields = append(emptyFields, "providers.oidc.issuer-url")
		}

		if o.ClientID == "" {
			emptyFields = append(emptyFields, "providers.oidc.client-id")
		}

		if o.ClientSecret == "" && !o.PkceRequired {
			emptyFields = append(emptyFields, "providers.oidc.client-secret")
		}

		return errors.New(strings.Join(emptyFields, ", ") + " must be set")
	}

	return nil
}