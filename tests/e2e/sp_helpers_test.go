//go:build e2e

package e2e_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const (
	defaultContainerSPURL = "http://localhost:8082/api/v1alpha1"
	defaultNATSURL        = "nats://localhost:4222"
	natsStatusSubject     = "dcm.container" // flat subject, NOT hierarchical
)

var (
	containerSPBaseURL string
	containerSPReady   bool
	natsURL            string
)

func initContainerSP() {
	containerSPBaseURL = os.Getenv("DCM_CONTAINER_SP_URL")
	if containerSPBaseURL == "" {
		containerSPBaseURL = defaultContainerSPURL
	}
	containerSPBaseURL = strings.TrimRight(containerSPBaseURL, "/")

	natsURL = os.Getenv("DCM_NATS_URL")
	if natsURL == "" {
		natsURL = defaultNATSURL
	}

	resp, err := httpClient.Get(containerSPBaseURL + "/containers/health")
	if err != nil {
		GinkgoWriter.Printf("Container SP not reachable at %s: %v — SP tests will be skipped\n", containerSPBaseURL, err)
		return
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		GinkgoWriter.Printf("Container SP health returned %d — SP tests will be skipped\n", resp.StatusCode)
		return
	}
	containerSPReady = true
	GinkgoWriter.Printf("Container SP ready at %s\n", containerSPBaseURL)
}

func requireContainerSP() {
	if !containerSPReady {
		Skip("Container SP not available (deploy with --k8s-container-service-provider and publish port 8082)")
	}
}

// doContainerSPRequest sends a request to the container SP's direct API.
func doContainerSPRequest(method, path string, body string) (*http.Response, error) {
	url := containerSPBaseURL + path

	var reqBody io.Reader
	if body != "" {
		reqBody = strings.NewReader(body)
	}

	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		return nil, err
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}

	return httpClient.Do(req)
}

// createTestContainer creates a container via the SP API and returns the parsed response body.
// It fails the test if creation doesn't succeed.
func createTestContainer(spec string) map[string]interface{} {
	resp, err := doContainerSPRequest(http.MethodPost, "/containers", spec)
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode).To(SatisfyAny(Equal(http.StatusOK), Equal(http.StatusCreated)),
		"create container failed with status %d", resp.StatusCode)

	var body map[string]interface{}
	decodeJSON(resp, &body)
	Expect(body).To(HaveKey("id"))
	return body
}

// deleteTestContainer removes a container by ID, ignoring 404 (already gone).
func deleteTestContainer(id string) {
	resp, err := doContainerSPRequest(http.MethodDelete, "/containers/"+id, "")
	if err != nil {
		GinkgoWriter.Printf("Warning: cleanup DELETE failed for container %s: %v\n", id, err)
		return
	}
	resp.Body.Close()
}

// containerSpec returns a JSON body for creating a test container per the OpenAPI schema.
func containerSpec(name, imageRef string, ports bool) string {
	spec := map[string]interface{}{
		"service_type": "container",
		"metadata":     map[string]interface{}{"name": name},
		"image":        map[string]interface{}{"reference": imageRef},
		"resources": map[string]interface{}{
			"cpu":    map[string]interface{}{"min": 1, "max": 1},
			"memory": map[string]interface{}{"min": "128MB", "max": "256MB"},
		},
	}
	if ports {
		spec["network"] = map[string]interface{}{
			"ports": []map[string]interface{}{
				{"container_port": 80, "visibility": "internal"},
			},
		}
	}
	body := map[string]interface{}{"spec": spec}
	data, _ := json.Marshal(body)
	return string(data)
}

// uniqueName generates a short unique name for test containers.
func uniqueName(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano()%1000000)
}

// --- NATS helpers --------------------------------------------------------- //

// CloudEvent represents the status event published by the container SP.
type CloudEvent struct {
	SpecVersion     string                 `json:"specversion"`
	ID              string                 `json:"id"`
	Source          string                 `json:"source"`
	Type            string                 `json:"type"`
	Time            string                 `json:"time"`
	DataContentType string                 `json:"datacontenttype"`
	Data            map[string]interface{} `json:"data"`
}

// NATSCollector subscribes to a NATS subject and collects messages.
type NATSCollector struct {
	conn   *nats.Conn
	sub    *nats.Subscription
	mu     sync.Mutex
	events []CloudEvent
}

func newNATSCollector(subject string) *NATSCollector {
	conn, err := nats.Connect(natsURL,
		nats.Timeout(5*time.Second),
		nats.Name("dcm-e2e-test"),
	)
	Expect(err).NotTo(HaveOccurred(), "failed to connect to NATS at %s", natsURL)

	c := &NATSCollector{conn: conn}

	sub, err := conn.Subscribe(subject, func(msg *nats.Msg) {
		var evt CloudEvent
		if err := json.Unmarshal(msg.Data, &evt); err != nil {
			GinkgoWriter.Printf("Warning: failed to parse NATS message: %v\n", err)
			return
		}
		c.mu.Lock()
		c.events = append(c.events, evt)
		c.mu.Unlock()
	})
	Expect(err).NotTo(HaveOccurred(), "failed to subscribe to %s", subject)
	c.sub = sub

	return c
}

func (c *NATSCollector) Close() {
	if c.sub != nil {
		_ = c.sub.Unsubscribe()
	}
	if c.conn != nil {
		c.conn.Close()
	}
}

// EventsForInstance returns events matching the given instance ID.
func (c *NATSCollector) EventsForInstance(instanceID string) []CloudEvent {
	c.mu.Lock()
	defer c.mu.Unlock()
	var out []CloudEvent
	for _, e := range c.events {
		if id, ok := e.Data["id"].(string); ok && id == instanceID {
			out = append(out, e)
		}
	}
	return out
}

// WaitForStatus polls until an event with the given instance ID and status appears.
func (c *NATSCollector) WaitForStatus(instanceID, status string, timeout time.Duration) CloudEvent {
	var matched CloudEvent
	Eventually(func() bool {
		for _, e := range c.EventsForInstance(instanceID) {
			if s, ok := e.Data["status"].(string); ok && s == status {
				matched = e
				return true
			}
		}
		return false
	}).WithTimeout(timeout).WithPolling(500 * time.Millisecond).Should(BeTrue(),
		fmt.Sprintf("timed out waiting for status %q on instance %s", status, instanceID))
	return matched
}
