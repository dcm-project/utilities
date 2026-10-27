//go:build e2e

package e2e_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("OSAC external-provider path", Label("osac", "external-provider", "environment-agent"), func() {
	It("creates an OSAC catalog instance through control-plane and environment-agent", func() {
		catalogItemID := os.Getenv("OSAC_EXTERNAL_CATALOG_ITEM_ID")
		if catalogItemID == "" {
			Skip("OSAC_EXTERNAL_CATALOG_ITEM_ID must identify a pre-seeded OSAC catalog item")
		}

		name := uniqueName("e2e-osac-external")
		payload, err := json.Marshal(map[string]interface{}{
			"display_name": name,
			"api_version":  "v1alpha1",
			"spec": map[string]interface{}{
				"catalog_item_id": catalogItemID,
				"user_values":     []interface{}{},
			},
		})
		Expect(err).NotTo(HaveOccurred())

		resp, err := doRequest(http.MethodPost, "/catalog-item-instances", string(payload))
		Expect(err).NotTo(HaveOccurred())
		body := readBody(resp)
		Expect(resp.StatusCode).To(SatisfyAny(Equal(http.StatusOK), Equal(http.StatusCreated)),
			"external OSAC create returned %d: %s", resp.StatusCode, string(body))

		var instance struct {
			UID         string   `json:"uid"`
			ResourceIDs []string `json:"resource_ids"`
		}
		Expect(json.Unmarshal(body, &instance)).To(Succeed())
		Expect(instance.UID).NotTo(BeEmpty())
		Expect(instance.ResourceIDs).NotTo(BeEmpty(), "external placement must return a resource ID")

		DeferCleanup(func() {
			cleanup, cleanupErr := doRequest(http.MethodDelete, "/catalog-item-instances/"+instance.UID, "")
			Expect(cleanupErr).NotTo(HaveOccurred(), "delete external OSAC catalog instance")
			defer cleanup.Body.Close()
			Expect(cleanup.StatusCode).To(SatisfyAny(Equal(http.StatusOK), Equal(http.StatusAccepted), Equal(http.StatusNoContent)))
		})

		resourceID := instance.ResourceIDs[0]
		readStatus := func() string {
			statusResp, statusErr := doRequest(http.MethodGet, "/service-type-instances/"+resourceID, "")
			if statusErr != nil {
				return ""
			}
			defer statusResp.Body.Close()
			if statusResp.StatusCode != http.StatusOK {
				return fmt.Sprintf("http-%d", statusResp.StatusCode)
			}
			var status struct {
				Status string `json:"status"`
			}
			if json.NewDecoder(statusResp.Body).Decode(&status) != nil {
				return ""
			}
			return strings.ToUpper(status.Status)
		}
		Eventually(readStatus, 30*time.Second, time.Second).ShouldNot(BeEmpty(),
			"external OSAC placement must expose a service-type-instance status")
		Consistently(readStatus, 15*time.Second, time.Second).ShouldNot(Equal("FAILED"),
			"external OSAC placement must not fail immediately due to provider request shape")
	})
})
