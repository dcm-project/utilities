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

const defaultAgentURL = "http://localhost:8081/api/v1alpha1"

type networkAgentProvider struct {
	ServiceType string `json:"service_type"`
	Status      string `json:"status"`
	Type        string `json:"type"`
}

type networkAgentProviderList struct {
	Results []networkAgentProvider `json:"results"`
}

type networkCRUDResource struct {
	PolicyID      string
	CatalogItemID string
	InstanceID    string
	ResourceID    string
	ServiceName   string
	Namespace     string
	NodePort      int
}

var _ = Describe("Network SP API", Label("sp", "network"), func() {
	var networkAgentName string

	Context("Phase A wiring", Label("lab-default"), Ordered, func() {
		It("E2E-01 verifies control-plane and embedded agent health", func() {
			assertControlPlaneHealth()
			assertEmbeddedAgentHealth()
			waitForEmbeddedNetworkProvider()
		})

		It("E2E-02 discovers a registered network agent", func() {
			override := os.Getenv("DCM_NETWORK_AGENT_NAME")
			networkAgentName = discoverAgentByServiceType("network", override)
			Expect(networkAgentName).NotTo(BeEmpty())
			GinkgoWriter.Printf("Selected network agent: %s\n", networkAgentName)
		})

		It("E2E-03 verifies the network service type schema", func() {
			serviceType := fetchNetworkServiceType()
			Expect(serviceType["service_type"]).To(Equal("network"))
			Expect(serviceType["api_version"]).To(Equal("v1alpha1"))

			spec, ok := serviceType["spec"].(map[string]interface{})
			Expect(ok).To(BeTrue(), "network service type must include a schema object")
			Expect(spec).To(HaveKey("ports"))
			Expect(spec).To(HaveKey("routing_level"))
			Expect(spec).To(HaveKey("endpoints"))
		})
	})

	Context("Phase B CRUD", Label("crud", "cluster"), func() {
		Context("ClusterIP inference", Label("clusterip"), Ordered, func() {
			var resource networkCRUDResource
			AfterAll(func() { cleanupNetworkResource(resource) })
			It("E2E-04 creates and reads a ClusterIP network", func() {
				agentName := discoverAgentByServiceType("network", os.Getenv("DCM_NETWORK_AGENT_NAME"))
				resource = createNetworkResource(agentName, networkSpecOptions{
					namePrefix: "e2e-network-clusterip", selectorKey: "app", selectorVal: "e2e-network",
				})
				waitForNetworkReady(resource.ResourceID)
				assertNetworkService(resource, "ClusterIP", "", true, "")
				assertNetworkInstance(resource, agentName, "ready")
			})
			It("E2E-05 deletes the ClusterIP network and its Kubernetes Service", func() {
				deleteNetworkResource(resource)
				resource = networkCRUDResource{}
			})
		})

		Context("NodePort inference", Label("nodeport"), Ordered, func() {
			var resource networkCRUDResource
			AfterAll(func() { cleanupNetworkResource(resource) })
			It("E2E-09 creates and reads a NodePort network", func() {
				agentName := discoverAgentByServiceType("network", os.Getenv("DCM_NETWORK_AGENT_NAME"))
				resource = createNetworkResource(agentName, networkSpecOptions{
					namePrefix: "e2e-network-nodeport", nodePort: 30080, selectorKey: "app", selectorVal: "e2e-network",
				})
				waitForNetworkReady(resource.ResourceID)
				assertNetworkService(resource, "NodePort", "", false, "http")
				assertNetworkInstance(resource, agentName, "ready")
			})
			It("deletes the NodePort network and its Kubernetes Service", func() {
				deleteNetworkResource(resource)
				resource = networkCRUDResource{}
			})
		})

		It("E2E-06 creates a LoadBalancer network without node ports", Label("no-lb-controller"), func() {
			skipIfMetalLBIsReady()
			agentName := discoverAgentByServiceType("network", os.Getenv("DCM_NETWORK_AGENT_NAME"))
			resource := createNetworkResource(agentName, networkSpecOptions{
				namePrefix: "e2e-network-loadbalancer", routingLevel: "network", selectorKey: "app", selectorVal: "e2e-network",
			})
			defer cleanupNetworkResource(resource)
			waitForNetworkPending(resource.ResourceID)
			assertNetworkService(resource, "LoadBalancer", "", false, "")
			assertNetworkServiceHasNoExternalIP(resource)
			assertNetworkInstance(resource, agentName, "pending")
		})

		It("E2E-12 creates a MetalLB-backed LoadBalancer network", Label("requires-metallb"), func() {
			requireMetalLB()
			agentName := discoverAgentByServiceType("network", os.Getenv("DCM_NETWORK_AGENT_NAME"))
			resource := createNetworkResource(agentName, networkSpecOptions{
				namePrefix: "e2e-network-metallb", routingLevel: "network", selectorKey: "app", selectorVal: "e2e-network",
			})
			defer cleanupNetworkResource(resource)
			waitForNetworkReady(resource.ResourceID)
			assertNetworkService(resource, "LoadBalancer", "", false, "")
			assertNetworkServiceHasExternalIP(resource)
			assertNetworkInstance(resource, agentName, "ready")
		})

		It("E2E-10 creates a LoadBalancer network with a node port", Label("loadbalancer"), func() {
			agentName := discoverAgentByServiceType("network", os.Getenv("DCM_NETWORK_AGENT_NAME"))
			resource := createNetworkResource(agentName, networkSpecOptions{
				namePrefix: "e2e-network-loadbalancer-np", routingLevel: "network", nodePort: 30081, selectorKey: "app", selectorVal: "e2e-network",
			})
			defer cleanupNetworkResource(resource)
			metalLBReady := metalLBIsReady()
			if metalLBReady {
				waitForNetworkReady(resource.ResourceID)
			} else {
				waitForNetworkPending(resource.ResourceID)
			}
			assertNetworkService(resource, "LoadBalancer", "", false, "http")
			if metalLBReady {
				assertNetworkServiceHasExternalIP(resource)
				assertNetworkInstance(resource, agentName, "ready")
			} else {
				assertNetworkInstance(resource, agentName, "pending")
			}
		})

		It("E2E-13 creates a headless ClusterIP network", Label("headless"), func() {
			agentName := discoverAgentByServiceType("network", os.Getenv("DCM_NETWORK_AGENT_NAME"))
			resource := createNetworkResource(agentName, networkSpecOptions{
				namePrefix: "e2e-network-headless", clusterIP: "None", selectorKey: "app", selectorVal: "e2e-network",
			})
			defer cleanupNetworkResource(resource)
			waitForNetworkReady(resource.ResourceID)
			assertNetworkService(resource, "ClusterIP", "None", false, "")
			assertNetworkInstance(resource, agentName, "ready")
		})

		It("E2E-11 rejects application routing and does not create a Service", Label("contract", "unsupported"), func() {
			agentName := discoverAgentByServiceType("network", os.Getenv("DCM_NETWORK_AGENT_NAME"))
			resource := createNetworkResource(agentName, networkSpecOptions{
				namePrefix: "e2e-network-unsupported", routingLevel: "application", selectorKey: "app", selectorVal: "e2e-network",
			})
			defer cleanupNetworkResource(resource)
			waitForNetworkFailed(resource.ResourceID)
			assertNetworkServiceAbsent(resource)
		})

		It("E2E-08 rejects an incomplete catalog-item instance request", Label("contract"), func() {
			resp, err := doRequest(http.MethodPost, "/catalog-item-instances", "{}")
			Expect(err).NotTo(HaveOccurred())
			expectRFC9457Problem(resp, problemDetailExpectation{
				Status: http.StatusBadRequest,
			})
		})
	})
})

type networkSpecOptions struct {
	namePrefix   string
	clusterIP    string
	nodePort     int
	routingLevel string
	selectorKey  string
	selectorVal  string
}

func createNetworkResource(agentName string, opts networkSpecOptions) networkCRUDResource {
	GinkgoHelper()
	resource := networkCRUDResource{
		ServiceName: uniqueName(opts.namePrefix),
		Namespace:   networkTestNamespace(),
		NodePort:    opts.nodePort,
	}
	resource.PolicyID = createNetworkPolicy(agentName)
	resource.CatalogItemID = createNetworkCatalogItem(resource.ServiceName, opts)
	resource.InstanceID, resource.ResourceID = createNetworkInstance(resource, opts)
	// The embedded provider uses the control-plane resource ID as the
	// Kubernetes Service name, even when metadata.name was supplied.
	resource.ServiceName = resource.ResourceID
	return resource
}

func createNetworkPolicy(agentName string) string {
	GinkgoHelper()
	packageName := fmt.Sprintf("e2e_network_%d", time.Now().UnixNano()%1000000)
	payload := map[string]interface{}{
		"display_name": uniqueName("e2e-network-policy"),
		"policy_type":  "GLOBAL",
		"priority":     100,
		"description":  "E2E network Phase B routing policy",
		"rego_code":    fmt.Sprintf("package %s\n\nmain := {\"selected_agent\": \"%s\"}", packageName, agentName),
	}
	resp := postNetworkJSON("/policies", payload)
	Expect(resp.StatusCode).To(Equal(http.StatusCreated))
	var policy map[string]interface{}
	decodeJSON(resp, &policy)
	id, _ := policy["id"].(string)
	Expect(id).NotTo(BeEmpty())
	return id
}

func createNetworkCatalogItem(serviceName string, opts networkSpecOptions) string {
	GinkgoHelper()
	fields := []map[string]interface{}{
		{"path": "metadata.name", "display_name": "Name", "editable": true, "default": serviceName},
		{"path": "ports", "display_name": "Ports", "editable": true, "default": networkPorts()},
		{"path": "provider_hints.kubernetes.selector", "display_name": "Selector", "editable": true, "default": networkSelector(opts)},
	}
	if opts.clusterIP != "" {
		fields = append(fields, map[string]interface{}{
			"path": "provider_hints.kubernetes.cluster_ip", "display_name": "Cluster IP", "editable": true, "default": opts.clusterIP,
		})
	}
	if opts.nodePort != 0 {
		fields = append(fields, map[string]interface{}{
			"path": "provider_hints.kubernetes.node_ports", "display_name": "Node ports", "editable": true, "default": map[string]int{"http": opts.nodePort},
		})
	}
	if opts.routingLevel != "" {
		fields = append(fields, map[string]interface{}{
			"path": "routing_level", "display_name": "Routing level", "editable": true, "default": opts.routingLevel,
		})
	}
	payload := map[string]interface{}{
		"api_version":  "v1alpha1",
		"display_name": uniqueName("e2e-network-catalog"),
		"spec": map[string]interface{}{
			"resources": []interface{}{
				map[string]interface{}{"name": "main", "service_type": "network", "fields": fields},
			},
		},
	}
	resp := postNetworkJSON("/catalog-items", payload)
	Expect(resp.StatusCode).To(Equal(http.StatusCreated))
	var catalog map[string]interface{}
	decodeJSON(resp, &catalog)
	id, _ := catalog["uid"].(string)
	Expect(id).NotTo(BeEmpty())
	return id
}

func createNetworkInstance(resource networkCRUDResource, opts networkSpecOptions) (string, string) {
	GinkgoHelper()
	payload := map[string]interface{}{
		"api_version":  "v1alpha1",
		"display_name": resource.ServiceName,
		"spec": map[string]interface{}{
			"catalog_item_id": resource.CatalogItemID,
			"user_values":     networkUserValues(resource.ServiceName, opts),
		},
	}
	beforeSTIs := listServiceTypeInstanceIDs()
	resp := postNetworkJSON("/catalog-item-instances", payload)
	Expect(resp.StatusCode).To(Equal(http.StatusCreated))
	var instance map[string]interface{}
	decodeJSON(resp, &instance)
	instanceID, _ := instance["uid"].(string)
	Expect(instanceID).NotTo(BeEmpty())
	resourceID := resolveResourceIDAfterCreate(instance, beforeSTIs)
	Expect(resourceID).NotTo(BeEmpty())
	return instanceID, resourceID
}

func networkPorts() []map[string]interface{} {
	return []map[string]interface{}{{"name": "http", "port": 80, "target_port": 8080, "protocol": "TCP"}}
}

func networkSelector(opts networkSpecOptions) map[string]string {
	return map[string]string{opts.selectorKey: opts.selectorVal}
}

func networkUserValues(serviceName string, opts networkSpecOptions) []map[string]interface{} {
	values := []map[string]interface{}{
		{"path": "metadata.name", "value": serviceName, "resource": "main"},
		{"path": "ports", "value": networkPorts(), "resource": "main"},
		{"path": "provider_hints.kubernetes.selector", "value": networkSelector(opts), "resource": "main"},
	}
	if opts.clusterIP != "" {
		values = append(values, map[string]interface{}{
			"path": "provider_hints.kubernetes.cluster_ip", "value": opts.clusterIP, "resource": "main",
		})
	}
	if opts.nodePort != 0 {
		values = append(values, map[string]interface{}{
			"path": "provider_hints.kubernetes.node_ports", "value": map[string]int{"http": opts.nodePort}, "resource": "main",
		})
	}
	if opts.routingLevel != "" {
		values = append(values, map[string]interface{}{
			"path": "routing_level", "value": opts.routingLevel, "resource": "main",
		})
	}
	return values
}

func postNetworkJSON(path string, payload interface{}) *http.Response {
	GinkgoHelper()
	body, err := json.Marshal(payload)
	Expect(err).NotTo(HaveOccurred())
	resp, err := doRequest(http.MethodPost, path, string(body))
	Expect(err).NotTo(HaveOccurred())
	return resp
}

func waitForNetworkReady(resourceID string) {
	GinkgoHelper()
	waitForNetworkStatus(resourceID, "ready")
}

func waitForNetworkPending(resourceID string) {
	GinkgoHelper()
	waitForNetworkStatus(resourceID, "pending")
}

func waitForNetworkStatus(resourceID, expected string) {
	GinkgoHelper()
	Eventually(func() interface{} {
		resp, err := doRequest(http.MethodGet, "/service-type-instances/"+resourceID, "")
		if err != nil || resp == nil {
			return "unavailable"
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return "unavailable"
		}
		var body map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			return "invalid-response"
		}
		status, _ := body["status"].(string)
		statusMessage, _ := body["status_message"].(string)
		if status == "failed" && expected != "failed" {
			return StopTrying(fmt.Sprintf("network resource %s failed: %s", resourceID, statusMessage))
		}
		if status == "running" && expected == "failed" {
			return StopTrying(fmt.Sprintf("unsupported network routing unexpectedly reached RUNNING: %s", resourceID))
		}
		return status
	}).WithTimeout(120 * time.Second).WithPolling(3 * time.Second).Should(Equal(expected))
}

func waitForNetworkFailed(resourceID string) {
	GinkgoHelper()
	waitForNetworkStatus(resourceID, "failed")
}

func assertNetworkInstance(resource networkCRUDResource, agentName, expectedStatus string) {
	GinkgoHelper()
	resp, err := doRequest(http.MethodGet, "/service-type-instances/"+resource.ResourceID, "")
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusOK))
	var body map[string]interface{}
	decodeJSON(resp, &body)
	Expect(body["status"]).To(Equal(expectedStatus))
	Expect(body["agent_name"]).To(Equal(agentName))
}

func assertNetworkService(resource networkCRUDResource, wantType, wantClusterIP string, requireAssignedClusterIP bool, wantNodePortName string) {
	GinkgoHelper()
	requireKubectl()
	var out string
	Eventually(func() bool {
		var err error
		out, err = runKubectlInNamespace(resource.Namespace, "get", "service", resource.ServiceName, "-o", "json")
		return err == nil
	}).WithTimeout(30*time.Second).WithPolling(2*time.Second).Should(BeTrue(),
		"Kubernetes Service %s was not readable; last kubectl output: %s", resource.ServiceName, out)
	var service map[string]interface{}
	Expect(json.Unmarshal([]byte(out), &service)).To(Succeed())
	metadata, ok := service["metadata"].(map[string]interface{})
	Expect(ok).To(BeTrue())
	labels, ok := metadata["labels"].(map[string]interface{})
	Expect(ok).To(BeTrue())
	Expect(labels["dcm.project/managed-by"]).To(Equal("dcm"))
	Expect(labels["dcm.project/dcm-service-type"]).To(Equal("network"))
	Expect(labels["dcm.project/dcm-instance-id"]).To(Equal(resource.ResourceID))
	spec, ok := service["spec"].(map[string]interface{})
	Expect(ok).To(BeTrue())
	Expect(spec["type"]).To(Equal(wantType))
	Expect(spec["selector"]).To(Equal(map[string]interface{}{"app": "e2e-network"}))
	if wantClusterIP != "" {
		Expect(spec["clusterIP"]).To(Equal(wantClusterIP))
	}
	if requireAssignedClusterIP {
		clusterIP, ok := spec["clusterIP"].(string)
		Expect(ok).To(BeTrue())
		Expect(clusterIP).NotTo(BeEmpty())
		Expect(clusterIP).NotTo(Equal("None"))
	}
	ports, ok := spec["ports"].([]interface{})
	Expect(ok).To(BeTrue())
	Expect(ports).To(HaveLen(1))
	port, ok := ports[0].(map[string]interface{})
	Expect(ok).To(BeTrue())
	Expect(port["name"]).To(Equal("http"))
	Expect(port["protocol"]).To(Equal("TCP"))
	Expect(port["port"]).To(Equal(float64(80)))
	Expect(port["targetPort"]).To(Equal(float64(8080)))
	if wantNodePortName != "" {
		Expect(port["name"]).To(Equal(wantNodePortName))
		Expect(port["nodePort"]).To(Equal(float64(resource.NodePort)))
	}
}

func assertNetworkServiceAbsent(resource networkCRUDResource) {
	GinkgoHelper()
	requireKubectl()
	Eventually(func() string {
		out, _ := runKubectlInNamespace(resource.Namespace, "get", "service", resource.ServiceName,
			"-o", "name", "--ignore-not-found")
		return strings.TrimSpace(out)
	}).WithTimeout(30*time.Second).WithPolling(2*time.Second).Should(BeEmpty(),
		"unsupported network request must not create Kubernetes Service %s", resource.ServiceName)
}

func assertNetworkServiceHasNoExternalIP(resource networkCRUDResource) {
	GinkgoHelper()
	out, err := runKubectlInNamespace(resource.Namespace, "get", "service", resource.ServiceName, "-o", "json")
	Expect(err).NotTo(HaveOccurred(), "kubectl output: %s", out)
	var service map[string]interface{}
	Expect(json.Unmarshal([]byte(out), &service)).To(Succeed())
	status, ok := service["status"].(map[string]interface{})
	Expect(ok).To(BeTrue())
	ingress, ok := status["loadBalancer"].(map[string]interface{})
	if !ok {
		return
	}
	if values, ok := ingress["ingress"].([]interface{}); ok {
		Expect(values).To(BeEmpty(), "LoadBalancer Service must not have an external address on the laptop stack")
	}
}

func assertNetworkServiceHasExternalIP(resource networkCRUDResource) {
	GinkgoHelper()
	requireKubectl()
	out, err := runKubectlInNamespace(resource.Namespace, "get", "service", resource.ServiceName, "-o", "json")
	Expect(err).NotTo(HaveOccurred(), "kubectl output: %s", out)
	var service map[string]interface{}
	Expect(json.Unmarshal([]byte(out), &service)).To(Succeed())
	status, ok := service["status"].(map[string]interface{})
	Expect(ok).To(BeTrue())
	loadBalancer, ok := status["loadBalancer"].(map[string]interface{})
	Expect(ok).To(BeTrue())
	ingress, ok := loadBalancer["ingress"].([]interface{})
	Expect(ok).To(BeTrue())
	Expect(ingress).NotTo(BeEmpty(), "MetalLB LoadBalancer Service must have an external address")
	entry, ok := ingress[0].(map[string]interface{})
	Expect(ok).To(BeTrue())
	Expect(entry["ip"] != "" || entry["hostname"] != "").To(BeTrue(),
		"external address must contain an IP or hostname")
}

func requireMetalLB() {
	GinkgoHelper()
	if !metalLBIsReady() {
		Skip("MetalLB controller is not ready")
	}
}

func skipIfMetalLBIsReady() {
	GinkgoHelper()
	if metalLBIsReady() {
		Skip("MetalLB is installed; this case requires a cluster without a load-balancer controller")
	}
}

func metalLBIsReady() bool {
	out, err := runKubectlInNamespace("metallb-system", "get", "deployment", "controller", "-o", "json")
	if err != nil {
		return false
	}
	var deployment map[string]interface{}
	if json.Unmarshal([]byte(out), &deployment) != nil {
		return false
	}
	status, ok := deployment["status"].(map[string]interface{})
	if !ok {
		return false
	}
	available, _ := status["availableReplicas"].(float64)
	return available > 0
}

func deleteNetworkResource(resource networkCRUDResource) {
	GinkgoHelper()
	if resource.InstanceID == "" {
		return
	}
	resp, err := doRequest(http.MethodDelete, "/catalog-item-instances/"+resource.InstanceID, "")
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode).To(BeNumerically(">=", http.StatusOK))
	Expect(resp.StatusCode).To(BeNumerically("<", http.StatusMultipleChoices))
	resp.Body.Close()
	Eventually(func() int {
		resp, err := doRequest(http.MethodGet, "/catalog-item-instances/"+resource.InstanceID, "")
		if err != nil || resp == nil {
			return 0
		}
		defer resp.Body.Close()
		return resp.StatusCode
	}).WithTimeout(60 * time.Second).WithPolling(3 * time.Second).Should(Equal(http.StatusNotFound))
	Eventually(func() string {
		resp, err := doRequest(http.MethodGet, "/service-type-instances/"+resource.ResourceID, "")
		if err != nil || resp == nil {
			return ""
		}
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusNotFound {
			return ""
		}
		return fmt.Sprintf("status-%d", resp.StatusCode)
	}).WithTimeout(60*time.Second).WithPolling(3*time.Second).Should(BeEmpty(),
		"service-type-instance %s should be removed", resource.ResourceID)
	Eventually(func() string {
		out, _ := runKubectlInNamespace(resource.Namespace, "get", "service", resource.ServiceName,
			"-o", "name", "--ignore-not-found")
		return strings.TrimSpace(out)
	}).WithTimeout(60*time.Second).WithPolling(3*time.Second).Should(BeEmpty(),
		"Kubernetes Service %s should be removed", resource.ServiceName)
	if resource.CatalogItemID != "" {
		resp, err := doRequest(http.MethodDelete, "/catalog-items/"+resource.CatalogItemID, "")
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode).To(BeNumerically(">=", http.StatusOK))
		Expect(resp.StatusCode).To(BeNumerically("<", http.StatusMultipleChoices))
		resp.Body.Close()
	}
	if resource.PolicyID != "" {
		resp, err := doRequest(http.MethodDelete, "/policies/"+resource.PolicyID, "")
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode).To(BeNumerically(">=", http.StatusOK))
		Expect(resp.StatusCode).To(BeNumerically("<", http.StatusMultipleChoices))
		resp.Body.Close()
	}
}

func cleanupNetworkResource(resource networkCRUDResource) {
	GinkgoHelper()
	if resource.InstanceID != "" {
		resp, err := doRequest(http.MethodDelete, "/catalog-item-instances/"+resource.InstanceID, "")
		if err == nil && resp != nil {
			resp.Body.Close()
		}
	}
	if resource.CatalogItemID != "" {
		resp, err := doRequest(http.MethodDelete, "/catalog-items/"+resource.CatalogItemID, "")
		if err == nil && resp != nil {
			resp.Body.Close()
		}
	}
	if resource.PolicyID != "" {
		resp, err := doRequest(http.MethodDelete, "/policies/"+resource.PolicyID, "")
		if err == nil && resp != nil {
			resp.Body.Close()
		}
	}
}

func networkTestNamespace() string {
	if namespace := os.Getenv("SP_K8S_NAMESPACE"); namespace != "" {
		return namespace
	}
	return "default"
}

func assertControlPlaneHealth() {
	resp, err := doRequest(http.MethodGet, "/health", "")
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusOK))
	var body map[string]interface{}
	decodeJSON(resp, &body)
	Expect(body).To(HaveKeyWithValue("status", "ok"))
}

func assertEmbeddedAgentHealth() {
	resp, err := networkAgentRequest(http.MethodGet, "/health")
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusOK))
	var body map[string]interface{}
	decodeJSON(resp, &body)
	Expect(body).To(HaveKeyWithValue("status", "healthy"))
	Expect(body).To(HaveKeyWithValue("path", "health"))
}

func waitForEmbeddedNetworkProvider() {
	Eventually(func() bool {
		resp, err := networkAgentRequest(http.MethodGet, "/providers")
		if err != nil || resp == nil {
			return false
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return false
		}

		var body networkAgentProviderList
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			return false
		}
		for _, provider := range body.Results {
			if provider.ServiceType == "network" &&
				provider.Type == "embedded" && provider.Status == "Ready" {
				return true
			}
		}
		return false
	}).WithTimeout(30*time.Second).WithPolling(2*time.Second).Should(BeTrue(),
		"embedded network provider should be registered and Ready")
}

func networkAgentRequest(method, path string) (*http.Response, error) {
	baseURL := os.Getenv("DCM_AGENT_URL")
	if baseURL == "" {
		baseURL = defaultAgentURL
	}
	url := strings.TrimRight(baseURL, "/") + path
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return nil, err
	}
	return httpClient.Do(req)
}

func fetchNetworkServiceType() map[string]interface{} {
	resp, err := doRequest(http.MethodGet, "/service-types", "")
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusOK))

	var body map[string]interface{}
	decodeJSON(resp, &body)
	results, ok := body["results"].([]interface{})
	Expect(ok).To(BeTrue(), "service-types response must contain results")
	for _, raw := range results {
		serviceType, ok := raw.(map[string]interface{})
		Expect(ok).To(BeTrue(), "service type entry should be an object")
		if serviceType["service_type"] == "network" {
			return serviceType
		}
	}

	Fail("network service type was not advertised by the control-plane")
	return nil
}
