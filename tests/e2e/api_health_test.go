//go:build e2e

package e2e_test

import (
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Health Endpoints", Label("smoke"), func() {
	healthEndpoints := map[string]string{
		"providers": "/health/providers",
		"catalog":   "/health/catalog",
		"policies":  "/health/policies",
		"placement": "/health/placement",
	}

	for serviceName, path := range healthEndpoints {
		It("reports healthy "+serviceName, func() {
			resp, err := doRequest(http.MethodGet, path, "")
			Expect(err).NotTo(HaveOccurred())

			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			var body map[string]interface{}
			decodeJSON(resp, &body)
			Expect(body).To(HaveKeyWithValue("status", BeElementOf("ok", "healthy")))
		})
	}
})
