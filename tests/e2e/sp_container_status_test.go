//go:build e2e

package e2e_test

import (
	"fmt"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Container SP Status Events", Label("sp", "container", "nats"), func() {
	BeforeEach(func() {
		requireContainerSP()
	})

	Context("CloudEvent format", Ordered, func() {
		var collector *NATSCollector
		var containerID string

		BeforeAll(func() {
			collector = newNATSCollector(natsStatusSubject)
		})

		AfterAll(func() {
			collector.Close()
			if containerID != "" {
				deleteTestContainer(containerID)
			}
		})

		It("publishes a CloudEvent on container creation", func() {
			name := uniqueName("e2e-ce")
			body := createTestContainer(containerSpec(name, "docker.io/library/nginx:alpine", false))
			containerID = body["id"].(string)

			Eventually(func() int {
				return len(collector.EventsForInstance(containerID))
			}).WithTimeout(30 * time.Second).WithPolling(1 * time.Second).Should(BeNumerically(">", 0),
				"expected at least one NATS event for container %s", containerID)

			events := collector.EventsForInstance(containerID)
			evt := events[0]

			By("validating CloudEvent required fields")
			Expect(evt.SpecVersion).To(Equal("1.0"))
			Expect(evt.ID).NotTo(BeEmpty())
			Expect(evt.Source).To(HavePrefix("dcm/providers/"), "source should be dcm/providers/<providerName>")
			Expect(evt.Type).NotTo(BeEmpty())
			Expect(evt.Time).NotTo(BeEmpty())
			Expect(evt.DataContentType).To(Equal("application/json"))
		})
	})

	Context("healthy container lifecycle", Ordered, func() {
		var collector *NATSCollector
		var containerID string

		BeforeAll(func() {
			collector = newNATSCollector(natsStatusSubject)
		})

		AfterAll(func() {
			collector.Close()
			if containerID != "" {
				deleteTestContainer(containerID)
			}
		})

		It("transitions to RUNNING and emits status events", func() {
			name := uniqueName("e2e-run")
			body := createTestContainer(containerSpec(name, "docker.io/library/nginx:alpine", false))
			containerID = body["id"].(string)

			By("waiting for RUNNING status via NATS")
			evt := collector.WaitForStatus(containerID, "RUNNING", 60*time.Second)
			Expect(evt.Data["status"]).To(Equal("RUNNING"))
			Expect(evt.Data["id"]).To(Equal(containerID))

			By("confirming GET also shows RUNNING")
			resp, err := doContainerSPRequest(http.MethodGet, "/containers/"+containerID, "")
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			var getBody map[string]interface{}
			decodeJSON(resp, &getBody)
			Expect(getBody["status"]).To(Equal("RUNNING"))
		})

		It("includes PENDING in event history before RUNNING", func() {
			events := collector.EventsForInstance(containerID)
			statuses := make([]string, 0, len(events))
			for _, e := range events {
				if s, ok := e.Data["status"].(string); ok {
					statuses = append(statuses, s)
				}
			}
			Expect(statuses).To(ContainElement("PENDING"), "should see PENDING before RUNNING")
			Expect(statuses).To(ContainElement("RUNNING"), "should see RUNNING")
		})
	})

	Context("image pull failure", Ordered, func() {
		var collector *NATSCollector
		var containerID string

		BeforeAll(func() {
			collector = newNATSCollector(natsStatusSubject)
		})

		AfterAll(func() {
			collector.Close()
			if containerID != "" {
				deleteTestContainer(containerID)
			}
		})

		// The SP maps ImagePullBackOff → PENDING (not FAILED).
		// This is by design: the status informer sees it as a transient waiting state.
		It("reports PENDING for an invalid image (ImagePullBackOff)", func() {
			name := uniqueName("e2e-bad")
			badImage := fmt.Sprintf("quay.io/nonexistent/image-%d:fake", time.Now().UnixNano())
			body := createTestContainer(containerSpec(name, badImage, false))
			containerID = body["id"].(string)

			By("waiting for a PENDING event")
			evt := collector.WaitForStatus(containerID, "PENDING", 60*time.Second)
			Expect(evt.Data["status"]).To(Equal("PENDING"))

			By("confirming it stays PENDING and does not transition to RUNNING")
			Consistently(func() string {
				events := collector.EventsForInstance(containerID)
				for _, e := range events {
					if s, _ := e.Data["status"].(string); s == "RUNNING" {
						return "RUNNING"
					}
				}
				return "PENDING"
			}).WithTimeout(15 * time.Second).WithPolling(3 * time.Second).Should(Equal("PENDING"))
		})
	})

	Context("initial status", Ordered, func() {
		var collector *NATSCollector
		var containerID string

		BeforeAll(func() {
			collector = newNATSCollector(natsStatusSubject)
		})

		AfterAll(func() {
			collector.Close()
			if containerID != "" {
				deleteTestContainer(containerID)
			}
		})

		It("emits PENDING as the initial status", func() {
			name := uniqueName("e2e-init")
			body := createTestContainer(containerSpec(name, "docker.io/library/nginx:alpine", false))
			containerID = body["id"].(string)

			Eventually(func() int {
				return len(collector.EventsForInstance(containerID))
			}).WithTimeout(15 * time.Second).WithPolling(1 * time.Second).Should(BeNumerically(">", 0))

			events := collector.EventsForInstance(containerID)
			Expect(events[0].Data["status"]).To(Equal("PENDING"))
		})
	})

	Context("delete status event", Ordered, func() {
		var collector *NATSCollector
		var containerID string

		BeforeAll(func() {
			collector = newNATSCollector(natsStatusSubject)
		})

		AfterAll(func() {
			collector.Close()
		})

		It("creates a container, waits for RUNNING, then deletes", func() {
			name := uniqueName("e2e-del")
			body := createTestContainer(containerSpec(name, "docker.io/library/nginx:alpine", false))
			containerID = body["id"].(string)

			collector.WaitForStatus(containerID, "RUNNING", 60*time.Second)

			resp, err := doContainerSPRequest(http.MethodDelete, "/containers/"+containerID, "")
			Expect(err).NotTo(HaveOccurred())
			resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(http.StatusNoContent))

			By("waiting for a DELETED or TERMINATED status event")
			Eventually(func() bool {
				for _, e := range collector.EventsForInstance(containerID) {
					s, _ := e.Data["status"].(string)
					if s == "DELETED" || s == "TERMINATED" || s == "STOPPED" {
						return true
					}
				}
				return false
			}).WithTimeout(30 * time.Second).WithPolling(2 * time.Second).Should(BeTrue(),
				"expected a terminal status event after deletion")
		})
	})

	Context("concurrent containers", Ordered, func() {
		var collector *NATSCollector
		var containerIDs []string

		BeforeAll(func() {
			collector = newNATSCollector(natsStatusSubject)
		})

		AfterAll(func() {
			collector.Close()
			for _, id := range containerIDs {
				deleteTestContainer(id)
			}
		})

		It("emits independent event streams for each container", func() {
			const count = 3
			for i := 0; i < count; i++ {
				name := uniqueName(fmt.Sprintf("e2e-con%d", i))
				body := createTestContainer(containerSpec(name, "docker.io/library/nginx:alpine", false))
				containerIDs = append(containerIDs, body["id"].(string))
			}

			for _, id := range containerIDs {
				Eventually(func() int {
					return len(collector.EventsForInstance(id))
				}).WithTimeout(60 * time.Second).WithPolling(2 * time.Second).Should(BeNumerically(">", 0),
					"expected at least one event for container %s", id)
			}

			By("verifying events are isolated per instance")
			for _, id := range containerIDs {
				events := collector.EventsForInstance(id)
				for _, e := range events {
					Expect(e.Data["id"]).To(Equal(id),
						"event for container %s contains wrong instance ID", id)
				}
			}
		})
	})
})
