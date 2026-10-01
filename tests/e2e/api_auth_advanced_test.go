//go:build e2e

package e2e_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func requireAuthTest() {
	if !authEnabled {
		Skip("authentication is disabled")
	}
}

func authRequest(token, method, endpoint string) *http.Response {
	req, err := http.NewRequest(method, endpoint, nil)
	Expect(err).NotTo(HaveOccurred())
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := unauthenticatedClient.Do(req)
	Expect(err).NotTo(HaveOccurred())
	return resp
}

func tokenClaims(token string) (map[string]interface{}, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("token is not a JWT")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, err
	}
	claims := map[string]interface{}{}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, err
	}
	return claims, nil
}

func tokenWithClaims(token string, mutate func(map[string]interface{})) string {
	parts := strings.Split(token, ".")
	claims, err := tokenClaims(token)
	Expect(err).NotTo(HaveOccurred())
	mutate(claims)
	payload, err := json.Marshal(claims)
	Expect(err).NotTo(HaveOccurred())
	return parts[0] + "." + base64.RawURLEncoding.EncodeToString(payload) + "." + parts[2]
}

func tokenStringClaim(token, name string) (string, error) {
	claims, err := tokenClaims(token)
	if err != nil {
		return "", err
	}
	value, ok := claims[name].(string)
	if !ok || value == "" {
		return "", fmt.Errorf("JWT claim %q is missing or not a string", name)
	}
	return value, nil
}

func tokenExpiry(token string) (time.Time, error) {
	claims, err := tokenClaims(token)
	if err != nil {
		return time.Time{}, err
	}
	expiry, ok := claims["exp"].(float64)
	if !ok {
		return time.Time{}, fmt.Errorf("JWT exp claim is missing or not numeric")
	}
	return time.Unix(int64(expiry), 0), nil
}

func tokenKeyID(token string) (string, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return "", fmt.Errorf("token is not a JWT")
	}
	header, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return "", fmt.Errorf("decode JWT header: %w", err)
	}
	var values map[string]interface{}
	if err := json.Unmarshal(header, &values); err != nil {
		return "", fmt.Errorf("decode JWT header JSON: %w", err)
	}
	keyID, ok := values["kid"].(string)
	if !ok || keyID == "" {
		return "", fmt.Errorf("JWT kid header is missing")
	}
	return keyID, nil
}

func publishedKeyIDs() ([]string, error) {
	jwksURL := os.Getenv("DCM_AUTH_JWKS_URL")
	if jwksURL != "" {
		return fetchPublishedKeyIDs(jwksURL)
	}
	issuer := authTokens.settings.tokenIssuerURL
	if issuer == "" {
		issuer = authTokens.settings.issuerURL
	}
	response, err := unauthenticatedClient.Get(strings.TrimRight(issuer, "/") + "/.well-known/openid-configuration")
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	var discovery struct {
		JWKSURI string `json:"jwks_uri"`
	}
	if err := json.NewDecoder(response.Body).Decode(&discovery); err != nil {
		return nil, err
	}
	return fetchPublishedKeyIDs(discovery.JWKSURI)
}

func fetchPublishedKeyIDs(jwksURL string) ([]string, error) {
	response, err := unauthenticatedClient.Get(jwksURL)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("JWKS endpoint returned HTTP %d", response.StatusCode)
	}
	var jwks struct {
		Keys []struct {
			KeyID string `json:"kid"`
		} `json:"keys"`
	}
	if err := json.NewDecoder(response.Body).Decode(&jwks); err != nil {
		return nil, err
	}
	keyIDs := make([]string, 0, len(jwks.Keys))
	for _, key := range jwks.Keys {
		keyIDs = append(keyIDs, key.KeyID)
	}
	return keyIDs, nil
}

func tokenHasAudience(token, expected string) (bool, error) {
	claims, err := tokenClaims(token)
	if err != nil {
		return false, err
	}
	switch audience := claims["aud"].(type) {
	case string:
		return audience == expected, nil
	case []interface{}:
		for _, item := range audience {
			if item == expected {
				return true, nil
			}
		}
	}
	return false, nil
}

func expectRejected(name, token string) {
	By(name)
	resp := authRequest(token, http.MethodGet, gatewayBaseURL+"/catalog-items")
	defer resp.Body.Close()
	Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized), name)
}

func expectRedactedLogs(source, command, token string) {
	output, err := exec.Command("sh", "-c", command).CombinedOutput()
	Expect(err).NotTo(HaveOccurred(), source)
	Expect(strings.TrimSpace(string(output))).NotTo(BeEmpty(), "%s log source was empty", source)
	Expect(string(output)).NotTo(ContainSubstring(token), source)
	Expect(string(output)).NotTo(MatchRegexp(`[A-Za-z0-9_-]{20,}\.[A-Za-z0-9_-]{20,}\.[A-Za-z0-9_-]{20,}`), source)
	Expect(string(output)).NotTo(MatchRegexp(`(?i)Bearer[[:space:]]+[A-Za-z0-9._~-]{20,}`), source)
}

func waitForTokenEndpoint() {
	issuer := os.Getenv("DCM_AUTH_TOKEN_ISSUER_URL")
	if issuer == "" {
		issuer = os.Getenv("DCM_AUTH_ISSUER_URL")
	}
	Eventually(func() error {
		resp, err := unauthenticatedClient.Get(strings.TrimRight(issuer, "/") + "/.well-known/openid-configuration")
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("token endpoint discovery returned HTTP %d", resp.StatusCode)
		}
		return nil
	}).WithTimeout(90 * time.Second).WithPolling(2 * time.Second).Should(Succeed())
}

func waitForRHDHProxy(proxyURL string) {
	Eventually(func() error {
		request, err := http.NewRequest(http.MethodGet, strings.TrimRight(proxyURL, "/")+"/catalog-items", nil)
		if err != nil {
			return err
		}
		response, err := unauthenticatedClient.Do(request)
		if err != nil {
			return err
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusUnauthorized {
			return fmt.Errorf("RHDH proxy returned HTTP %d; expected 401 while checking readiness", response.StatusCode)
		}
		return nil
	}).WithTimeout(90*time.Second).WithPolling(2*time.Second).Should(Succeed(),
		"RHDH proxy should become available")
}

func waitForKeycloakAdmin(admin *keycloakAdminClient) {
	Eventually(func() error {
		_, err := admin.realmID(context.Background())
		return err
	}).WithTimeout(90*time.Second).WithPolling(2*time.Second).Should(Succeed(),
		"RHBK Admin API should become available")
}

var _ = Describe("DCM authentication advanced E2E", Ordered, ContinueOnFailure, Label("auth", "security", "advanced-auth"), func() {
	It("TC-42 survives an RHBK restart", Label("restart"), func() {
		requireAuthTest()
		command := os.Getenv("DCM_AUTH_RESTART_COMMAND")
		if command == "" {
			Skip("set DCM_AUTH_RESTART_COMMAND for the target deployment")
		}
		oldToken, err := authTokens.Token(context.Background())
		Expect(err).NotTo(HaveOccurred())
		resp := authRequest(oldToken, http.MethodGet, gatewayBaseURL+"/catalog-items")
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		resp.Body.Close()

		cmd := exec.Command("sh", "-c", command)
		cmd.Stdout = GinkgoWriter
		cmd.Stderr = GinkgoWriter
		Expect(cmd.Run()).To(Succeed())
		waitForTokenEndpoint()
		Eventually(func() int {
			resp, err := unauthenticatedClient.Get(gatewayBaseURL + "/health")
			if err != nil {
				return 0
			}
			defer resp.Body.Close()
			return resp.StatusCode
		}).WithTimeout(60 * time.Second).Should(Equal(http.StatusOK))
		authTokens.mu.Lock()
		authTokens.token = ""
		authTokens.expiresAt = time.Time{}
		authTokens.mu.Unlock()
		newToken, err := authTokens.Token(context.Background())
		Expect(err).NotTo(HaveOccurred())
		resp = authRequest(newToken, http.MethodGet, gatewayBaseURL+"/catalog-items")
		defer resp.Body.Close()
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
	})

	It("TC-43 rejects token forwarding without a valid RHDH session", Label("proxy"), func() {
		requireAuthTest()
		proxyURL := os.Getenv("DCM_AUTH_PROXY_URL")
		if proxyURL == "" {
			Skip("set DCM_AUTH_PROXY_URL for the RHDH proxy")
		}
		waitForRHDHProxy(proxyURL)
		token, err := authTokens.Token(context.Background())
		Expect(err).NotTo(HaveOccurred())
		request, err := http.NewRequest(http.MethodGet, strings.TrimRight(proxyURL, "/")+"/catalog-items", nil)
		Expect(err).NotTo(HaveOccurred())
		resp, err := unauthenticatedClient.Do(request)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
		resp.Body.Close()
		request, err = http.NewRequest(http.MethodGet, strings.TrimRight(proxyURL, "/")+"/catalog-items", nil)
		Expect(err).NotTo(HaveOccurred())
		request.Header.Set("X-DCM-OIDC-Token", token)
		resp, err = unauthenticatedClient.Do(request)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
		resp.Body.Close()
		sessionToken := os.Getenv("DCM_AUTH_PROXY_SESSION_TOKEN")
		if sessionToken == "" {
			Skip("set DCM_AUTH_PROXY_SESSION_TOKEN for the valid RHDH session check")
		}
		request, err = http.NewRequest(http.MethodGet, strings.TrimRight(proxyURL, "/")+"/catalog-items", nil)
		Expect(err).NotTo(HaveOccurred())
		request.Header.Set("Authorization", "Bearer "+sessionToken)
		request.Header.Set("X-DCM-OIDC-Token", token)
		resp, err = unauthenticatedClient.Do(request)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode).To(BeNumerically(">=", http.StatusOK))
		Expect(resp.StatusCode).To(BeNumerically("<", http.StatusMultipleChoices))
		resp.Body.Close()
	})

	It("TC-44 rejects invalid JWT claims", Label("negative"), func() {
		requireAuthTest()
		ctx := context.Background()
		admin, err := loadKeycloakAdminClient()
		if err != nil {
			Skip(err.Error())
		}
		waitForKeycloakAdmin(admin)
		testClient, err := admin.createPublicClient(ctx, 1)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() { Expect(admin.delete(ctx, "/clients/"+testClient.ID)).To(Succeed()) })

		wrongAudience, err := admin.issueUserToken(ctx, testClient.ClientID)
		Expect(err).NotTo(HaveOccurred())
		expectedAudience := os.Getenv("DCM_AUTH_AUDIENCE")
		if expectedAudience == "" {
			expectedAudience = "dcm-api"
		}
		hasAudience, err := tokenHasAudience(wrongAudience, expectedAudience)
		Expect(err).NotTo(HaveOccurred())
		Expect(hasAudience).To(BeFalse(), "wrong-audience token unexpectedly contains %q", expectedAudience)

		Expect(admin.addAudienceMapper(ctx, testClient.ID, expectedAudience)).To(Succeed())
		expired, err := admin.issueUserToken(ctx, testClient.ClientID)
		Expect(err).NotTo(HaveOccurred())
		expiry, err := tokenExpiry(expired)
		Expect(err).NotTo(HaveOccurred())
		Eventually(func() bool { return time.Now().After(expiry.Add(time.Second)) }).WithTimeout(10 * time.Second).Should(BeTrue())

		valid, err := authTokens.Token(context.Background())
		Expect(err).NotTo(HaveOccurred())
		foreignIssuer := admin.adminToken
		issuer, err := tokenStringClaim(foreignIssuer, "iss")
		Expect(err).NotTo(HaveOccurred())
		Expect(issuer).NotTo(Equal(authTokens.settings.issuerURL))

		cases := []struct {
			name  string
			token string
		}{
			{name: "malformed", token: "not-a-jwt"},
			{name: "invalid signature", token: tokenWithClaims(valid, func(c map[string]interface{}) { c["sub"] = "tampered" })},
			{name: "expired", token: expired},
			{name: "foreign issuer", token: foreignIssuer},
			{name: "wrong audience", token: wrongAudience},
		}
		for _, testCase := range cases {
			expectRejected(testCase.name, testCase.token)
		}
	})

	It("TC-45 preserves valid tokens across signing-key rotation", Label("rotation", "disruptive"), func() {
		requireAuthTest()
		ctx := context.Background()
		admin, err := loadKeycloakAdminClient()
		if err != nil {
			Skip(err.Error())
		}
		waitForKeycloakAdmin(admin)
		testClient, err := admin.createPublicClient(ctx, 300)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() { Expect(admin.delete(ctx, "/clients/"+testClient.ID)).To(Succeed()) })
		audience := os.Getenv("DCM_AUTH_AUDIENCE")
		if audience == "" {
			audience = "dcm-api"
		}
		Expect(admin.addAudienceMapper(ctx, testClient.ID, audience)).To(Succeed())

		oldToken, err := admin.issueUserToken(ctx, testClient.ClientID)
		Expect(err).NotTo(HaveOccurred())
		oldKeyID, err := tokenKeyID(oldToken)
		Expect(err).NotTo(HaveOccurred())
		oldExpiry, err := tokenExpiry(oldToken)
		Expect(err).NotTo(HaveOccurred())
		oldResponse := authRequest(oldToken, http.MethodGet, gatewayBaseURL+"/catalog-items")
		Expect(oldResponse.StatusCode).To(Equal(http.StatusOK))
		oldResponse.Body.Close()

		componentID, err := admin.createSigningProvider(ctx)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() { Expect(admin.delete(ctx, "/components/"+componentID)).To(Succeed()) })

		var newToken string
		Eventually(func() bool {
			candidate, issueErr := admin.issueUserToken(ctx, testClient.ClientID)
			if issueErr != nil {
				return false
			}
			newKeyID, keyErr := tokenKeyID(candidate)
			if keyErr != nil || newKeyID == oldKeyID {
				return false
			}
			response := authRequest(candidate, http.MethodGet, gatewayBaseURL+"/catalog-items")
			defer response.Body.Close()
			if response.StatusCode != http.StatusOK {
				return false
			}
			newToken = candidate
			return true
		}).WithTimeout(60 * time.Second).WithPolling(2 * time.Second).Should(BeTrue())
		Expect(newToken).NotTo(BeEmpty())

		keyIDs, err := publishedKeyIDs()
		Expect(err).NotTo(HaveOccurred())
		Expect(keyIDs).To(ContainElement(oldKeyID), "old signing key must remain published during overlap")
		Expect(time.Now()).To(BeTemporally("<", oldExpiry), "old token expired before overlap validation")
		oldResponse = authRequest(oldToken, http.MethodGet, gatewayBaseURL+"/catalog-items")
		defer oldResponse.Body.Close()
		Expect(oldResponse.StatusCode).To(Equal(http.StatusOK), "old unexpired token signed by a published key must remain valid")
	})

	It("TC-46 does not expose bearer tokens in configured logs", Label("redaction"), func() {
		requireAuthTest()
		token, err := authTokens.Token(context.Background())
		Expect(err).NotTo(HaveOccurred())
		resp := authRequest(token, http.MethodGet, gatewayBaseURL+"/catalog-items")
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		resp.Body.Close()

		proxyURL := os.Getenv("DCM_AUTH_PROXY_URL")
		sessionToken := os.Getenv("DCM_AUTH_PROXY_SESSION_TOKEN")
		if proxyURL != "" && sessionToken != "" {
			request, requestErr := http.NewRequest(http.MethodGet, strings.TrimRight(proxyURL, "/")+"/catalog-items", nil)
			Expect(requestErr).NotTo(HaveOccurred())
			request.Header.Set("Authorization", "Bearer "+sessionToken)
			request.Header.Set("X-DCM-OIDC-Token", token)
			resp, requestErr = unauthenticatedClient.Do(request)
			Expect(requestErr).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(BeNumerically(">=", http.StatusOK))
			Expect(resp.StatusCode).To(BeNumerically("<", http.StatusMultipleChoices))
			resp.Body.Close()
		}
		logCommands := map[string]string{
			"DCM":  os.Getenv("DCM_AUTH_DCM_LOG_COMMAND"),
			"RHDH": os.Getenv("DCM_AUTH_RHDH_LOG_COMMAND"),
		}
		checked := 0
		for source, command := range logCommands {
			if command == "" {
				continue
			}
			expectRedactedLogs(source, command, token)
			checked++
		}
		if checked > 0 {
			return
		}
		files := strings.Split(os.Getenv("DCM_AUTH_LOG_FILES"), ":")
		if len(files) == 0 || files[0] == "" {
			Skip("set DCM_AUTH_DCM_LOG_COMMAND, DCM_AUTH_RHDH_LOG_COMMAND, or DCM_AUTH_LOG_FILES")
		}
		for _, filename := range files {
			data, err := os.ReadFile(filename)
			Expect(err).NotTo(HaveOccurred(), filename)
			Expect(strings.TrimSpace(string(data))).NotTo(BeEmpty(), filename)
			Expect(string(data)).NotTo(ContainSubstring(token), filename)
			Expect(string(data)).NotTo(MatchRegexp(`[A-Za-z0-9_-]{20,}\.[A-Za-z0-9_-]{20,}\.[A-Za-z0-9_-]{20,}`), filename)
		}
	})
})
