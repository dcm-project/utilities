//go:build e2e

package e2e_test

import (
	"fmt"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Providers API", func() {
	Context("CRUD lifecycle", Ordered, func() {
		var providerID string
		providerName := fmt.Sprintf("e2e-test-provider-%d", time.Now().UnixNano())

		// Cleanup runs once after all ordered specs, catching any leftover provider.
		AfterAll(func() {
			if providerID != "" {
				resp, err := doRequest(http.MethodDelete, "/providers/"+providerID, "")
				if err != nil {
					GinkgoWriter.Printf("Warning: cleanup failed for provider %s: %v\n", providerID, err)
				}
				if resp != nil {
					resp.Body.Close()
				}
			}
		})

		It("creates a provider", func() {
			payload := fmt.Sprintf(`{
				"name": %q,
				"endpoint": "https://example.com/api",
				"service_type": "vm",
				"schema_version": "v1alpha1"
			}`, providerName)

			resp, err := doRequest(http.MethodPost, "/providers", payload)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusCreated))

			var body map[string]interface{}
			decodeJSON(resp, &body)
			Expect(body).To(HaveKey("id"))
			Expect(body["name"]).To(Equal(providerName))
			Expect(body["status"]).To(Equal("registered"))

			id, ok := body["id"].(string)
			Expect(ok).To(BeTrue(), "id should be a string")
			providerID = id
		})

		It("gets the provider by ID", func() {
			Expect(providerID).NotTo(BeEmpty(), "provider must be created first")

			resp, err := doRequest(http.MethodGet, "/providers/"+providerID, "")
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			var body map[string]interface{}
			decodeJSON(resp, &body)
			Expect(body["name"]).To(Equal(providerName))
			Expect(body["endpoint"]).To(Equal("https://example.com/api"))
		})

		It("lists providers and includes the created one", func() {
			Expect(providerID).NotTo(BeEmpty(), "provider must be created first")

			resp, err := doRequest(http.MethodGet, "/providers", "")
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			var body map[string]interface{}
			decodeJSON(resp, &body)
			Expect(body).To(HaveKey("providers"))

			providers, ok := body["providers"].([]interface{})
			Expect(ok).To(BeTrue())

			var found bool
			for _, p := range providers {
				provider, ok := p.(map[string]interface{})
				Expect(ok).To(BeTrue(), "provider entry should be a map")
				if provider["id"] == providerID {
					found = true
					break
				}
			}
			Expect(found).To(BeTrue(), "created provider should appear in list")
		})

		It("deletes the provider", func() {
			Expect(providerID).NotTo(BeEmpty(), "provider must be created first")

			resp, err := doRequest(http.MethodDelete, "/providers/"+providerID, "")
			Expect(err).NotTo(HaveOccurred())
			resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(http.StatusNoContent))

			By("confirming the provider is gone")
			deletedID := providerID
			providerID = ""
			resp, err = doRequest(http.MethodGet, "/providers/"+deletedID, "")
			Expect(err).NotTo(HaveOccurred())
			resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
		})
	})

	Context("error cases", func() {
		It("returns 404 for a non-existent provider", func() {
			resp, err := doRequest(http.MethodGet, "/providers/does-not-exist", "")
			Expect(err).NotTo(HaveOccurred())
			resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
		})
	})

	Context("idempotent registration", Ordered, func() {
		var originalID string
		providerName := fmt.Sprintf("e2e-idempotent-%d", time.Now().UnixNano())

		AfterAll(func() {
			if originalID != "" {
				resp, err := doRequest(http.MethodDelete, "/providers/"+originalID, "")
				if err != nil {
					GinkgoWriter.Printf("Warning: cleanup failed for provider %s: %v\n", originalID, err)
				}
				if resp != nil {
					resp.Body.Close()
				}
			}
		})

		It("creates the initial provider", func() {
			payload := fmt.Sprintf(`{
				"name": %q,
				"endpoint": "https://example.com/api/idempotent",
				"service_type": "vm",
				"schema_version": "v1alpha1"
			}`, providerName)

			resp, err := doRequest(http.MethodPost, "/providers", payload)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusCreated))

			var body map[string]interface{}
			decodeJSON(resp, &body)
			Expect(body["status"]).To(Equal("registered"))

			id, ok := body["id"].(string)
			Expect(ok).To(BeTrue(), "id should be a string")
			originalID = id
		})

		It("re-registers same name without ID and returns updated", func() { // ECOPROJECT-4344
			Expect(originalID).NotTo(BeEmpty(), "provider must be created first")

			payload := fmt.Sprintf(`{
				"name": %q,
				"endpoint": "https://example.com/api/idempotent-v2",
				"service_type": "vm",
				"schema_version": "v1alpha1"
			}`, providerName)

			resp, err := doRequest(http.MethodPost, "/providers", payload)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			var body map[string]interface{}
			decodeJSON(resp, &body)
			Expect(body["status"]).To(Equal("updated"))
			Expect(body["id"]).To(Equal(originalID), "provider ID should be preserved")
		})

		It("re-registers same name with same ID and returns updated", func() { // ECOPROJECT-4345
			Expect(originalID).NotTo(BeEmpty(), "provider must be created first")

			payload := fmt.Sprintf(`{
				"name": %q,
				"endpoint": "https://example.com/api/idempotent-v3",
				"service_type": "vm",
				"schema_version": "v1alpha1"
			}`, providerName)

			resp, err := doRequest(http.MethodPost, fmt.Sprintf("/providers?id=%s", originalID), payload)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			var body map[string]interface{}
			decodeJSON(resp, &body)
			Expect(body["status"]).To(Equal("updated"))
			Expect(body["id"]).To(Equal(originalID))
		})

		It("returns 409 when same name sent with different ID", func() { // ECOPROJECT-4346
			Expect(originalID).NotTo(BeEmpty(), "provider must be created first")

			payload := fmt.Sprintf(`{
				"name": %q,
				"endpoint": "https://example.com/api/conflict",
				"service_type": "vm",
				"schema_version": "v1alpha1"
			}`, providerName)

			resp, err := doRequest(http.MethodPost, "/providers?id=bogus-conflict-id-0099", payload)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusConflict))

			var body map[string]interface{}
			decodeJSON(resp, &body)
			Expect(body["type"]).To(Equal("conflict"))
			Expect(body["title"]).To(Equal("Resource conflict"))
			Expect(body["status"]).To(BeNumerically("==", 409))
		})

		It("returns 409 when new name sent with existing ID", func() { // ECOPROJECT-4347
			Expect(originalID).NotTo(BeEmpty(), "provider must be created first")

			differentName := fmt.Sprintf("e2e-idempotent-other-%d", time.Now().UnixNano())
			payload := fmt.Sprintf(`{
				"name": %q,
				"endpoint": "https://example.com/api/conflict",
				"service_type": "vm",
				"schema_version": "v1alpha1"
			}`, differentName)

			resp, err := doRequest(http.MethodPost, fmt.Sprintf("/providers?id=%s", originalID), payload)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusConflict))

			var body map[string]interface{}
			decodeJSON(resp, &body)
			Expect(body["type"]).To(Equal("conflict"))
			Expect(body["title"]).To(Equal("Resource conflict"))
			Expect(body["status"]).To(BeNumerically("==", 409))
		})
	})

	Context("PUT update", Ordered, func() {
		var providerID1, providerID2 string
		providerName1 := fmt.Sprintf("e2e-put-1-%d", time.Now().UnixNano())
		providerName2 := fmt.Sprintf("e2e-put-2-%d", time.Now().UnixNano())

		AfterAll(func() {
			for _, id := range []string{providerID1, providerID2} {
				if id != "" {
					resp, err := doRequest(http.MethodDelete, "/providers/"+id, "")
					if err != nil {
						GinkgoWriter.Printf("Warning: cleanup failed for provider %s: %v\n", id, err)
					}
					if resp != nil {
						resp.Body.Close()
					}
				}
			}
		})

		It("creates providers for PUT tests", func() {
			for _, tc := range []struct {
				name string
				id   *string
			}{
				{providerName1, &providerID1},
				{providerName2, &providerID2},
			} {
				payload := fmt.Sprintf(`{
					"name": %q,
					"endpoint": "https://example.com/api/put-test",
					"service_type": "vm",
					"schema_version": "v1alpha1"
				}`, tc.name)

				resp, err := doRequest(http.MethodPost, "/providers", payload)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(http.StatusCreated))

				var body map[string]interface{}
				decodeJSON(resp, &body)

				id, ok := body["id"].(string)
				Expect(ok).To(BeTrue(), "id should be a string")
				*tc.id = id
			}
		})

		It("updates a provider via PUT", func() { // ECOPROJECT-4348
			Expect(providerID1).NotTo(BeEmpty(), "provider must be created first")

			payload := fmt.Sprintf(`{
				"name": %q,
				"endpoint": "https://updated-put.example.com/api",
				"service_type": "container",
				"schema_version": "v1alpha1"
			}`, providerName1)

			resp, err := doRequest(http.MethodPut, "/providers/"+providerID1, payload)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			var body map[string]interface{}
			decodeJSON(resp, &body)
			Expect(body["endpoint"]).To(Equal("https://updated-put.example.com/api"))
			Expect(body["service_type"]).To(Equal("container"))
			Expect(body["name"]).To(Equal(providerName1))
		})

		It("returns 404 when updating non-existent provider", func() { // ECOPROJECT-4349
			payload := `{
				"name": "ghost-provider",
				"endpoint": "https://example.com/api",
				"service_type": "vm",
				"schema_version": "v1alpha1"
			}`

			resp, err := doRequest(http.MethodPut, "/providers/does-not-exist-00", payload)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusNotFound))

			var body map[string]interface{}
			decodeJSON(resp, &body)
			Expect(body["type"]).To(Equal("not-found"))
			Expect(body["title"]).To(Equal("Provider not found"))
		})

		It("returns 409 when PUT update causes name collision", func() { // ECOPROJECT-4350
			Expect(providerID2).NotTo(BeEmpty(), "second provider must be created first")

			payload := fmt.Sprintf(`{
				"name": %q,
				"endpoint": "https://example.com/api/put-test",
				"service_type": "vm",
				"schema_version": "v1alpha1"
			}`, providerName1)

			resp, err := doRequest(http.MethodPut, "/providers/"+providerID2, payload)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusConflict))

			var body map[string]interface{}
			decodeJSON(resp, &body)
			Expect(body["type"]).To(Equal("conflict"))
			Expect(body["title"]).To(Equal("Name conflict"))
			Expect(body["status"]).To(BeNumerically("==", 409))
		})
	})
})
