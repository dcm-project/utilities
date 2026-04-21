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
			GinkgoWriter.Printf("Create provider response: %v\n", body)
			Expect(body).To(HaveKey("id"))
			Expect(body["name"]).To(Equal(providerName))

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
})
