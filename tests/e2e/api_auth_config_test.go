//go:build e2e

package e2e_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLoadAuthSettings(t *testing.T) {
	t.Run("disabled by default", func(t *testing.T) {
		t.Setenv("DCM_AUTH_ENABLED", "false")
		settings, err := loadAuthSettings()
		if err != nil {
			t.Fatalf("loadAuthSettings() error = %v", err)
		}
		if settings.enabled {
			t.Fatal("expected authentication to be disabled")
		}
	})

	t.Run("enabled requires issuer and credentials", func(t *testing.T) {
		t.Setenv("DCM_AUTH_ENABLED", "true")
		t.Setenv("DCM_AUTH_ISSUER_URL", "")
		if _, err := loadAuthSettings(); err == nil {
			t.Fatal("expected missing issuer to fail")
		}
	})

	t.Run("static token does not require password credentials", func(t *testing.T) {
		t.Setenv("DCM_AUTH_ENABLED", "true")
		t.Setenv("DCM_AUTH_ISSUER_URL", "https://issuer.example/realms/test")
		t.Setenv("DCM_AUTH_TOKEN", "test-token")
		t.Setenv("DCM_AUTH_CLIENT_SECRET", "")
		t.Setenv("DCM_AUTH_USERNAME", "")
		t.Setenv("DCM_AUTH_PASSWORD", "")
		if _, err := loadAuthSettings(); err != nil {
			t.Fatalf("loadAuthSettings() error = %v", err)
		}
	})
}

func TestAuthTransportRefreshesAndInjectsBearerToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/realms/test/protocol/openid-connect/token":
			if err := request.ParseForm(); err != nil {
				t.Fatalf("ParseForm() error = %v", err)
			}
			if got := request.PostForm.Get("client_id"); got != "dcm-proxy" {
				t.Errorf("client_id = %q, want dcm-proxy", got)
			}
			writer.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(writer, `{"access_token":"test-token","expires_in":300}`)
		case "/api/v1alpha1/catalog-items":
			if got := request.Header.Get("Authorization"); got != "Bearer test-token" {
				t.Errorf("Authorization = %q, want Bearer test-token", got)
			}
			writer.WriteHeader(http.StatusOK)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	settings := authSettings{
		enabled:      true,
		issuerURL:    server.URL + "/realms/test",
		clientID:     "dcm-proxy",
		clientSecret: "secret",
		username:     "testuser",
		password:     "password",
	}
	provider := &authTokenProvider{
		settings: settings,
		client:   server.Client(),
	}
	client := &http.Client{
		Transport: &authTransport{base: server.Client().Transport, tokens: provider, origin: server.URL},
	}

	request, err := http.NewRequest(http.MethodGet, server.URL+"/api/v1alpha1/catalog-items", nil)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("client.Do() error = %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.StatusCode, http.StatusOK)
	}
}
