//go:build e2e

package e2e_test

import (
	"fmt"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Service Type Instances API", func() {
	Context("service_type filter", Ordered, func() {
		var containerProviderName string
		var catalogItemID, instanceID, resourceID string

		BeforeAll(func() {
			requireContainerSP()

			By("discovering the container provider")
			resp, err := doRequest(http.MethodGet, "/providers?type=container", "")
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			var provBody map[string]interface{}
			decodeJSON(resp, &provBody)
			providers, ok := provBody["providers"].([]interface{})
			Expect(ok).To(BeTrue())
			Expect(providers).NotTo(BeEmpty(), "no container providers registered")

			p := providers[0].(map[string]interface{})
			containerProviderName, _ = p["name"].(string)
			Expect(containerProviderName).NotTo(BeEmpty())
			GinkgoWriter.Printf("Using container provider: %s\n", containerProviderName)

			By("creating a catalog item for the container service type")
			catName := uniqueName("e2e-filter")
			catPayload := fmt.Sprintf(`{
				"api_version": "v1alpha1",
				"display_name": %q,
				"spec": {
					"service_type": "container",
					"fields": [
						{"path": "metadata.name", "display_name": "Container Name", "editable": true, "default": %q},
						{"path": "image.reference", "display_name": "Image", "editable": true, "default": "docker.io/library/nginx:alpine"},
						{"path": "resources.cpu.min", "editable": false, "default": 1},
						{"path": "resources.cpu.max", "editable": false, "default": 1},
						{"path": "resources.memory.min", "editable": false, "default": "128MB"},
						{"path": "resources.memory.max", "editable": false, "default": "256MB"}
					]
				}
			}`, catName, catName)

			resp, err = doRequest(http.MethodPost, "/catalog-items", catPayload)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusCreated))

			var catBody map[string]interface{}
			decodeJSON(resp, &catBody)
			catalogItemID, _ = catBody["uid"].(string)
			Expect(catalogItemID).NotTo(BeEmpty())
			GinkgoWriter.Printf("Created catalog item: %s\n", catalogItemID)

			By("creating a catalog item instance (relies on existing routing policy)")
			instName := uniqueName("e2e-filter-inst")
			instPayload := fmt.Sprintf(`{
				"api_version": "v1alpha1",
				"display_name": %q,
				"spec": {
					"catalog_item_id": %q,
					"user_values": [
						{"path": "metadata.name", "value": %q},
						{"path": "image.reference", "value": "docker.io/library/nginx:alpine"}
					]
				}
			}`, instName, catalogItemID, instName)

			resp, err = doRequest(http.MethodPost, "/catalog-item-instances", instPayload)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusCreated))

			var instBody map[string]interface{}
			decodeJSON(resp, &instBody)
			instanceID, _ = instBody["uid"].(string)
			Expect(instanceID).NotTo(BeEmpty())
			resourceID, _ = instBody["resource_id"].(string)
			Expect(resourceID).NotTo(BeEmpty(), "resource_id should be set synchronously by placement")
			GinkgoWriter.Printf("Created catalog-item-instance: %s (resource_id=%s)\n", instanceID, resourceID)

			By("waiting for the service-type-instance to be queryable")
			Eventually(func() int {
				r, e := doRequest(http.MethodGet, "/service-type-instances/"+resourceID, "")
				if e != nil {
					return 0
				}
				defer r.Body.Close()
				return r.StatusCode
			}).WithTimeout(30 * time.Second).WithPolling(2 * time.Second).Should(Equal(http.StatusOK))
		})

		AfterAll(func() {
			if instanceID != "" {
				resp, err := doRequest(http.MethodDelete, "/catalog-item-instances/"+instanceID, "")
				if err == nil && resp != nil {
					resp.Body.Close()
				}
				Eventually(func() int {
					r, e := doRequest(http.MethodGet, "/catalog-item-instances/"+instanceID, "")
					if e != nil {
						return 0
					}
					defer r.Body.Close()
					return r.StatusCode
				}).WithTimeout(60 * time.Second).WithPolling(3 * time.Second).Should(Equal(http.StatusNotFound))
			}
			if catalogItemID != "" {
				resp, err := doRequest(http.MethodDelete, "/catalog-items/"+catalogItemID, "")
				if err == nil && resp != nil {
					resp.Body.Close()
				}
			}
		})

		It("returns the instance when filtering by service_type=container", func() {
			resp, err := doRequest(http.MethodGet, "/service-type-instances?service_type=container", "")
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			var body map[string]interface{}
			decodeJSON(resp, &body)
			Expect(body).To(HaveKey("instances"))

			instances, ok := body["instances"].([]interface{})
			Expect(ok).To(BeTrue())

			var found bool
			for _, inst := range instances {
				i := inst.(map[string]interface{})
				if i["id"] == resourceID {
					found = true
					break
				}
			}
			Expect(found).To(BeTrue(),
				"instance %s should appear when filtering by service_type=container", resourceID)
		})

		It("excludes the instance when filtering by a different service_type", func() {
			resp, err := doRequest(http.MethodGet, "/service-type-instances?service_type=vm", "")
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			var body map[string]interface{}
			decodeJSON(resp, &body)
			Expect(body).To(HaveKey("instances"))

			instances, ok := body["instances"].([]interface{})
			Expect(ok).To(BeTrue())

			for _, inst := range instances {
				i := inst.(map[string]interface{})
				Expect(i["id"]).NotTo(Equal(resourceID),
					"container instance should not appear in service_type=vm results")
			}
		})

		It("returns empty list for non-existent service type", func() {
			resp, err := doRequest(http.MethodGet, "/service-type-instances?service_type=nonexistent-type", "")
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			var body map[string]interface{}
			decodeJSON(resp, &body)
			Expect(body).To(HaveKey("instances"))

			instances, ok := body["instances"].([]interface{})
			Expect(ok).To(BeTrue())
			Expect(instances).To(BeEmpty(),
				"filtering by a non-existent service_type should return zero results")
		})

		It("includes the instance in unfiltered list", func() {
			resp, err := doRequest(http.MethodGet, "/service-type-instances", "")
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			var body map[string]interface{}
			decodeJSON(resp, &body)
			Expect(body).To(HaveKey("instances"))

			instances, ok := body["instances"].([]interface{})
			Expect(ok).To(BeTrue())

			var found bool
			for _, inst := range instances {
				i := inst.(map[string]interface{})
				if i["id"] == resourceID {
					found = true
					break
				}
			}
			Expect(found).To(BeTrue(),
				"unfiltered list should include instance %s", resourceID)
		})

		It("combines service_type with provider filter", func() {
			resp, err := doRequest(http.MethodGet,
				"/service-type-instances?service_type=container&provider="+containerProviderName, "")
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			var body map[string]interface{}
			decodeJSON(resp, &body)
			Expect(body).To(HaveKey("instances"))

			instances, ok := body["instances"].([]interface{})
			Expect(ok).To(BeTrue())

			var found bool
			for _, inst := range instances {
				i := inst.(map[string]interface{})
				if i["id"] == resourceID {
					found = true
				}
				provName, _ := i["provider_name"].(string)
				Expect(provName).To(Equal(containerProviderName),
					"combined filter should only return instances from the specified provider")
			}
			Expect(found).To(BeTrue(),
				"combined filter should include our instance")
		})

		It("respects pagination with service_type filter", func() {
			resp, err := doRequest(http.MethodGet,
				"/service-type-instances?service_type=container&max_page_size=1", "")
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			var page1 map[string]interface{}
			decodeJSON(resp, &page1)
			Expect(page1).To(HaveKey("instances"))

			instances := page1["instances"].([]interface{})
			Expect(len(instances)).To(BeNumerically("<=", 1),
				"page size should be respected with filter")

			if token, ok := page1["next_page_token"].(string); ok && token != "" {
				resp2, err := doRequest(http.MethodGet,
					"/service-type-instances?service_type=container&max_page_size=1&page_token="+token, "")
				Expect(err).NotTo(HaveOccurred())
				Expect(resp2.StatusCode).To(Equal(http.StatusOK))

				var page2 map[string]interface{}
				decodeJSON(resp2, &page2)
				Expect(page2).To(HaveKey("instances"))
			}
		})
	})
})
