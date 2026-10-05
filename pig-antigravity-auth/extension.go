// Package antigravity registers the google-antigravity model provider: Google
// account OAuth against Antigravity's Cloud Code Assist endpoints, serving the
// Gemini 3, Claude and GPT-OSS models of an Antigravity subscription.
//
// It is the Go counterpart of @cortexkit/pi-antigravity-auth, for pig.
//
// Using Antigravity this way is outside Google's terms of service and can get
// an account suspended. It is an unofficial client.
package antigravity

import (
	"context"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
)

// Extension is the factory pig calls.
func Extension() *sdk.Extension {
	ext := sdk.New("pig-antigravity-auth")
	ext.RegisterProvider(providerID, sdk.ProviderConfig{
		"name":    "Google Antigravity",
		"baseUrl": "https://cloudcode-pa.googleapis.com",
		"api":     "google-generative-ai",
		"models":  providerModels(),
		"oauth": &sdk.OAuthProvider{
			Name:           "Google Antigravity",
			IsSubscription: true,
			Login:          login,
			RefreshToken: func(c sdk.OAuthCredentials) (sdk.OAuthCredentials, error) {
				return refreshCredentials(context.Background(), c)
			},
			GetAPIKey: apiKey,
		},
		"streamSimple": sdk.ProviderStreamSimpleFunc(streamSimple),
	})
	return ext
}
