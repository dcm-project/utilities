//go:build e2e

package e2e_test

import (
	"fmt"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// Tests for FLPATH-3426 / FLPATH-3427: the SPRM's NATS status consumer.
//
// The container SP publishes CloudEvents to NATS when Pod phase changes.
// The SPRM's StatusConsumer reads these events and calls UpdateStatus on
// its database, which is then reflected in the GET /service-type-instances API.
//
// These tests verify the full round-trip:
//   SP → NATS → SPRM consumer → DB → API response
//
// Existing tests cover adjacent concerns:
//   - sp_container_status_test.go: SP publishing behavior and NATS event format
//   - core_platform_test.go: single happy-path "reaches RUNNING" check
//
// This file focuses specifically on the status reader's behavior as observable
// through the gateway API.

var _ = Describe("Status Reader", Label("nats"), func() {
	Context("status propagation through gateway API", Ordered, func() {
		var containerProviderName string
		var policyID, catalogItemID, instanceID, resourceID string

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

			By("creating a routing policy to direct traffic to the container provider")
			polName := uniqueName("e2e-status-pol")
			pkgName := fmt.Sprintf("e2e_status_%d", time.Now().UnixNano()%1000000)
			polPayload := fmt.Sprintf(`{
				"display_name": %q,
				"policy_type": "GLOBAL",
				"priority": 100,
				"description": "E2E status reader test: route to container provider",
				"rego_code": "package %s\n\nmain := {\"selected_provider\": \"%s\"}"
			}`, polName, pkgName, containerProviderName)

			resp, err = doRequest(http.MethodPost, "/policies", polPayload)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusCreated))

			var polBody map[string]interface{}
			decodeJSON(resp, &polBody)
			policyID, _ = polBody["id"].(string)
			Expect(policyID).NotTo(BeEmpty())

			By("creating a catalog item")
			catName := uniqueName("e2e-status")
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

			By("creating a catalog item instance to trigger provisioning")
			instName := uniqueName("e2e-status-inst")
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
			GinkgoWriter.Printf("Status reader test: created instance %s (resource_id=%s)\n", instanceID, resourceID)
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
			if policyID != "" {
				resp, err := doRequest(http.MethodDelete, "/policies/"+policyID, "")
				if err == nil && resp != nil {
					resp.Body.Close()
				}
			}
		})

		It("has a status field immediately after creation", func() {
			resp, err := doRequest(http.MethodGet, "/service-type-instances/"+resourceID, "")
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			var body map[string]interface{}
			decodeJSON(resp, &body)
			Expect(body).To(HaveKey("status"), "instance should have a status field from creation")

			status, ok := body["status"].(string)
			Expect(ok).To(BeTrue(), "status should be a string")
			Expect(status).NotTo(BeEmpty(), "status should not be empty")
			GinkgoWriter.Printf("Initial status: %q\n", status)
		})

		It("reaches RUNNING status via NATS consumer update", func() {
			Eventually(func() string {
				resp, err := doRequest(http.MethodGet, "/service-type-instances/"+resourceID, "")
				if err != nil {
					return ""
				}
				defer resp.Body.Close()
				if resp.StatusCode != http.StatusOK {
					return ""
				}
				var body map[string]interface{}
				decodeJSON(resp, &body)
				s, _ := body["status"].(string)
				return s
			}).WithTimeout(120 * time.Second).WithPolling(3 * time.Second).Should(Equal("RUNNING"),
				"SPRM should reflect RUNNING after the NATS consumer processes the SP's status event")
		})

		It("returns status in the list endpoint", func() {
			resp, err := doRequest(http.MethodGet, "/service-type-instances?service_type=container", "")
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			var body map[string]interface{}
			decodeJSON(resp, &body)
			instances, ok := body["instances"].([]interface{})
			Expect(ok).To(BeTrue())

			var found bool
			for _, inst := range instances {
				i, ok := inst.(map[string]interface{})
				Expect(ok).To(BeTrue())
				if i["id"] == resourceID {
					found = true
					Expect(i).To(HaveKey("status"))
					Expect(i["status"]).To(Equal("RUNNING"),
						"list endpoint should show the same status as individual GET")
					break
				}
			}
			Expect(found).To(BeTrue(), "instance should appear in filtered list")
		})

		It("persists status across repeated reads", func() {
			for i := 0; i < 3; i++ {
				resp, err := doRequest(http.MethodGet, "/service-type-instances/"+resourceID, "")
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(http.StatusOK))

				var body map[string]interface{}
				decodeJSON(resp, &body)
				Expect(body["status"]).To(Equal("RUNNING"),
					"status should be consistent across read #%d", i+1)
			}
		})

		It("includes update_time reflecting the status change", func() {
			resp, err := doRequest(http.MethodGet, "/service-type-instances/"+resourceID, "")
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			var body map[string]interface{}
			decodeJSON(resp, &body)
			Expect(body).To(HaveKey("update_time"))
			Expect(body["update_time"]).NotTo(BeEmpty(),
				"update_time should be set after status consumer updates the record")
		})
	})

	Context("error status propagation", Ordered, func() {
		var policyID, catalogItemID, instanceID, resourceID string

		BeforeAll(func() {
			requireContainerSP()

			By("discovering the container provider for policy creation")
			resp, err := doRequest(http.MethodGet, "/providers?type=container", "")
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			var provBody map[string]interface{}
			decodeJSON(resp, &provBody)
			providers, ok := provBody["providers"].([]interface{})
			Expect(ok).To(BeTrue())
			Expect(providers).NotTo(BeEmpty())
			providerName, _ := providers[0].(map[string]interface{})["name"].(string)

			By("creating a routing policy")
			polName := uniqueName("e2e-badimg-pol")
			pkgName := fmt.Sprintf("e2e_badimg_%d", time.Now().UnixNano()%1000000)
			polPayload := fmt.Sprintf(`{
				"display_name": %q,
				"policy_type": "GLOBAL",
				"priority": 100,
				"description": "E2E bad image test: route to container provider",
				"rego_code": "package %s\n\nmain := {\"selected_provider\": \"%s\"}"
			}`, polName, pkgName, providerName)

			resp, err = doRequest(http.MethodPost, "/policies", polPayload)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusCreated))

			var polBody map[string]interface{}
			decodeJSON(resp, &polBody)
			policyID, _ = polBody["id"].(string)
			Expect(policyID).NotTo(BeEmpty())

			By("creating a catalog item with a bad image that will fail to pull")
			catName := uniqueName("e2e-badimg")
			badImage := fmt.Sprintf("quay.io/nonexistent/image-%d:fake", time.Now().UnixNano())
			catPayload := fmt.Sprintf(`{
				"api_version": "v1alpha1",
				"display_name": %q,
				"spec": {
					"service_type": "container",
					"fields": [
						{"path": "metadata.name", "display_name": "Container Name", "editable": true, "default": %q},
						{"path": "image.reference", "display_name": "Image", "editable": true, "default": %q},
						{"path": "resources.cpu.min", "editable": false, "default": 1},
						{"path": "resources.cpu.max", "editable": false, "default": 1},
						{"path": "resources.memory.min", "editable": false, "default": "128MB"},
						{"path": "resources.memory.max", "editable": false, "default": "256MB"}
					]
				}
			}`, catName, catName, badImage)

			resp, err = doRequest(http.MethodPost, "/catalog-items", catPayload)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusCreated))

			var catBody map[string]interface{}
			decodeJSON(resp, &catBody)
			catalogItemID, _ = catBody["uid"].(string)
			Expect(catalogItemID).NotTo(BeEmpty())

			By("creating an instance that will encounter ImagePullBackOff")
			instName := uniqueName("e2e-badimg-inst")
			instPayload := fmt.Sprintf(`{
				"api_version": "v1alpha1",
				"display_name": %q,
				"spec": {
					"catalog_item_id": %q,
					"user_values": [
						{"path": "metadata.name", "value": %q},
						{"path": "image.reference", "value": %q}
					]
				}
			}`, instName, catalogItemID, instName, badImage)

			resp, err = doRequest(http.MethodPost, "/catalog-item-instances", instPayload)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusCreated))

			var instBody map[string]interface{}
			decodeJSON(resp, &instBody)
			instanceID, _ = instBody["uid"].(string)
			Expect(instanceID).NotTo(BeEmpty())
			resourceID, _ = instBody["resource_id"].(string)
			Expect(resourceID).NotTo(BeEmpty())
			GinkgoWriter.Printf("Bad image test: created instance %s (resource_id=%s)\n", instanceID, resourceID)
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
			if policyID != "" {
				resp, err := doRequest(http.MethodDelete, "/policies/"+policyID, "")
				if err == nil && resp != nil {
					resp.Body.Close()
				}
			}
		})

		It("reflects PENDING status for a failing container", func() {
			Eventually(func() string {
				resp, err := doRequest(http.MethodGet, "/service-type-instances/"+resourceID, "")
				if err != nil {
					return ""
				}
				defer resp.Body.Close()
				if resp.StatusCode != http.StatusOK {
					return ""
				}
				var body map[string]interface{}
				decodeJSON(resp, &body)
				s, _ := body["status"].(string)
				return s
			}).WithTimeout(60 * time.Second).WithPolling(3 * time.Second).Should(Equal("PENDING"),
				"SPRM API should show PENDING when the SP reports ImagePullBackOff → PENDING")
		})

		It("does not transition to RUNNING for a bad image", func() {
			Consistently(func() string {
				resp, err := doRequest(http.MethodGet, "/service-type-instances/"+resourceID, "")
				if err != nil {
					return "error"
				}
				defer resp.Body.Close()
				if resp.StatusCode != http.StatusOK {
					return "error"
				}
				var body map[string]interface{}
				decodeJSON(resp, &body)
				s, _ := body["status"].(string)
				return s
			}).WithTimeout(15 * time.Second).WithPolling(3 * time.Second).ShouldNot(Equal("RUNNING"),
				"instance with invalid image should never reach RUNNING")
		})
	})

	Context("independent status updates for concurrent instances", Ordered, func() {
		var policyID, catalogItemID string
		var instanceIDs []string
		var resourceIDs []string

		const instanceCount = 3

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
			Expect(providers).NotTo(BeEmpty())
			providerName, _ := providers[0].(map[string]interface{})["name"].(string)

			By("creating a routing policy")
			polName := uniqueName("e2e-multi-pol")
			pkgName := fmt.Sprintf("e2e_multi_%d", time.Now().UnixNano()%1000000)
			polPayload := fmt.Sprintf(`{
				"display_name": %q,
				"policy_type": "GLOBAL",
				"priority": 100,
				"description": "E2E multi-instance test: route to container provider",
				"rego_code": "package %s\n\nmain := {\"selected_provider\": \"%s\"}"
			}`, polName, pkgName, providerName)

			resp, err = doRequest(http.MethodPost, "/policies", polPayload)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusCreated))

			var polBody map[string]interface{}
			decodeJSON(resp, &polBody)
			policyID, _ = polBody["id"].(string)
			Expect(policyID).NotTo(BeEmpty())

			By("creating a shared catalog item")
			catName := uniqueName("e2e-multi")
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

			By(fmt.Sprintf("creating %d instances to verify independent status tracking", instanceCount))
			for i := 0; i < instanceCount; i++ {
				instName := uniqueName(fmt.Sprintf("e2e-multi-%d", i))
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

				resp, err := doRequest(http.MethodPost, "/catalog-item-instances", instPayload)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(http.StatusCreated))

				var instBody map[string]interface{}
				decodeJSON(resp, &instBody)
				uid, _ := instBody["uid"].(string)
				rid, _ := instBody["resource_id"].(string)
				Expect(uid).NotTo(BeEmpty())
				Expect(rid).NotTo(BeEmpty())
				instanceIDs = append(instanceIDs, uid)
				resourceIDs = append(resourceIDs, rid)
				GinkgoWriter.Printf("Created instance %d: %s (resource_id=%s)\n", i, uid, rid)
			}
		})

		AfterAll(func() {
			for _, uid := range instanceIDs {
				resp, err := doRequest(http.MethodDelete, "/catalog-item-instances/"+uid, "")
				if err == nil && resp != nil {
					resp.Body.Close()
				}
			}
			for _, uid := range instanceIDs {
				Eventually(func() int {
					r, e := doRequest(http.MethodGet, "/catalog-item-instances/"+uid, "")
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
			if policyID != "" {
				resp, err := doRequest(http.MethodDelete, "/policies/"+policyID, "")
				if err == nil && resp != nil {
					resp.Body.Close()
				}
			}
		})

		It("each instance reaches RUNNING independently", func() {
			for i, rid := range resourceIDs {
				Eventually(func() string {
					resp, err := doRequest(http.MethodGet, "/service-type-instances/"+rid, "")
					if err != nil {
						return ""
					}
					defer resp.Body.Close()
					if resp.StatusCode != http.StatusOK {
						return ""
					}
					var body map[string]interface{}
					decodeJSON(resp, &body)
					s, _ := body["status"].(string)
					return s
				}).WithTimeout(120 * time.Second).WithPolling(3 * time.Second).Should(Equal("RUNNING"),
					"instance %d (resource_id=%s) should reach RUNNING via independent status update", i, rid)
			}
		})

		It("all instances show RUNNING simultaneously in list", func() {
			resp, err := doRequest(http.MethodGet, "/service-type-instances?service_type=container", "")
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			var body map[string]interface{}
			decodeJSON(resp, &body)
			instances, ok := body["instances"].([]interface{})
			Expect(ok).To(BeTrue())

			ridSet := make(map[string]bool, len(resourceIDs))
			for _, rid := range resourceIDs {
				ridSet[rid] = true
			}

			var foundCount int
			for _, inst := range instances {
				i, ok := inst.(map[string]interface{})
				Expect(ok).To(BeTrue())
				id, _ := i["id"].(string)
				if ridSet[id] {
					foundCount++
					Expect(i["status"]).To(Equal("RUNNING"),
						"instance %s should be RUNNING in list response", id)
				}
			}
			Expect(foundCount).To(Equal(instanceCount),
				"all %d test instances should appear in the list", instanceCount)
		})
	})
})
