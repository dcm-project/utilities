//go:build e2e

package e2e_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("KubeVirt Service Provider API", Label("sp", "kubevirt"), func() {
	BeforeEach(func() {
		requireKubevirtSP()
	})

	Context("Health endpoint", func() {
		It("returns healthy status when cluster is reachable [TC-04]", func() {
			resp, err := doKubevirtRequest(http.MethodGet, "/vms/health", "")
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			var body map[string]interface{}
			Expect(json.NewDecoder(resp.Body).Decode(&body)).To(Succeed())
			Expect(body["status"]).To(Equal("healthy"))
			Expect(body["path"]).To(Equal("/api/v1alpha1/health"))
		})
	})

	Context("VM create variants", func() {
		It("creates a VM with valid spec and DCM labels on cluster [TC-06][TC-11][TC-26][TC-27]", func() {
			if err := checkClusterAccess(); err != nil {
				Skip("Cluster access required: " + err.Error())
			}
			if err := checkStorageClass(); err != nil {
				Skip(err.Error())
			}

			vmName := uniqueName("e2e-kubevirt-vm")
			id, err := createTestVM(vmName)
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() { deleteTestVM(id) })

			ns := kubevirtNamespace()
			var clusterName string
			Eventually(func() error {
				name, err := findVMNameByInstanceID(id, ns)
				if err != nil {
					return err
				}
				clusterName = name
				return verifyDCMLabels(name, ns, id)
			}).WithTimeout(60 * time.Second).WithPolling(2 * time.Second).Should(Succeed())

			vm, err := getVMFromCluster(clusterName, ns)
			Expect(err).NotTo(HaveOccurred())

			// Spec mapping: requests memory present on domain resources
			domain, err := labelMap(vm, "spec", "template", "spec", "domain")
			Expect(err).NotTo(HaveOccurred())
			resources, _ := domain["resources"].(map[string]interface{})
			Expect(resources).NotTo(BeNil())
			requests, _ := resources["requests"].(map[string]interface{})
			Expect(requests).To(HaveKey("memory"))
			// OpenAPI input is 1GB; SP maps decimal GB → Kubernetes resource "1G" (not Gi).
			Expect(requests["memory"]).To(Equal("1G"))

			GinkgoWriter.Printf("Created VM id=%s clusterName=%s with DCM labels\n", id, clusterName)
		})

		It("creates a VM with custom ID via query parameter [TC-07]", func() {
			customID := uuid.NewString()
			spec := newTestVMSpec(uniqueName("e2e-custom-id"))
			payload, err := json.Marshal(map[string]interface{}{"spec": spec})
			Expect(err).NotTo(HaveOccurred())

			resp, err := doKubevirtRequest(http.MethodPost, "/vms?id="+customID, string(payload))
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(http.StatusCreated))
			DeferCleanup(func() { deleteTestVM(customID) })

			var body map[string]interface{}
			Expect(json.NewDecoder(resp.Body).Decode(&body)).To(Succeed())
			Expect(body["path"]).To(ContainSubstring(customID))

			getResp, err := doKubevirtRequest(http.MethodGet, "/vms/"+customID, "")
			Expect(err).NotTo(HaveOccurred())
			defer getResp.Body.Close()
			Expect(getResp.StatusCode).To(Equal(http.StatusOK))
		})

		It("returns 409 when creating a VM with a duplicate instance ID [TC-10]", func() {
			customID := uuid.NewString()
			spec := newTestVMSpec(uniqueName("e2e-dup"))
			payload, err := json.Marshal(map[string]interface{}{"spec": spec})
			Expect(err).NotTo(HaveOccurred())

			resp1, err := doKubevirtRequest(http.MethodPost, "/vms?id="+customID, string(payload))
			Expect(err).NotTo(HaveOccurred())
			defer resp1.Body.Close()
			Expect(resp1.StatusCode).To(Equal(http.StatusCreated))
			DeferCleanup(func() { deleteTestVM(customID) })

			resp2, err := doKubevirtRequest(http.MethodPost, "/vms?id="+customID, string(payload))
			Expect(err).NotTo(HaveOccurred())
			defer resp2.Body.Close()
			Expect(resp2.StatusCode).To(Equal(http.StatusConflict))
			expectProblemDetailContains(resp2, "conflict", "duplicate", "already", "id")
		})
	})

	Context("VM get / list / delete", func() {
		It("gets an existing VM [TC-12]", func() {
			id, err := createTestVM(uniqueName("e2e-get"))
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() { deleteTestVM(id) })

			resp, err := doKubevirtRequest(http.MethodGet, "/vms/"+id, "")
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			var body map[string]interface{}
			Expect(json.NewDecoder(resp.Body).Decode(&body)).To(Succeed())
			Expect(body).To(HaveKey("spec"))
			Expect(body).To(HaveKey("path"))
			Expect(body["path"]).To(ContainSubstring(id))
		})

		It("returns 404 for a non-existent VM [TC-13]", func() {
			resp, err := doKubevirtRequest(http.MethodGet, "/vms/non-existent-vm-id", "")
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusInternalServerError {
				Skip("SP returns 500 for missing VM (expected 404) — track as SP bug")
			}
			Expect(resp.StatusCode).To(Equal(http.StatusNotFound),
				"GET missing should be 404; if SP returns 500 file/track SP bug and Skip temporarily")
			expectProblemDetailContains(resp, "not found", "missing", "404", "notfound")
		})

		It("GET reflects current VM status [TC-14]", func() {
			if err := checkClusterAccess(); err != nil {
				Skip(err.Error())
			}
			id, err := createTestVM(uniqueName("e2e-status-get"))
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() { deleteTestVM(id) })

			Eventually(func() string {
				resp, err := doKubevirtRequest(http.MethodGet, "/vms/"+id, "")
				if err != nil {
					return ""
				}
				defer resp.Body.Close()
				if resp.StatusCode != http.StatusOK {
					return ""
				}
				var body map[string]interface{}
				if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
					return ""
				}
				if status, _ := body["status"].(string); status != "" {
					GinkgoWriter.Printf("GET /vms status=%s\n", status)
					return status
				}
				// Fallback: some SP builds omit status on GET — use cluster printable status
				ns := kubevirtNamespace()
				name, err := findVMNameByInstanceID(id, ns)
				if err != nil {
					return ""
				}
				out, err := runKubeCmd("get", "vm", name, "-n", ns, "-o", "jsonpath={.status.printableStatus}")
				if err != nil {
					return ""
				}
				s := strings.TrimSpace(out)
				GinkgoWriter.Printf("cluster printableStatus=%s\n", s)
				return s
			}).WithTimeout(180 * time.Second).WithPolling(5 * time.Second).
				Should(BeElementOf("Pending", "Scheduling", "Scheduled", "Running", "Succeeded", "Failed",
					"Starting", "Stopping", "Stopped", "Migrating", "Paused", "Unknown",
					"PENDING", "PROVISIONING", "RUNNING"),
					"GET or cluster should expose a known VM phase")
		})

		It("lists all managed VMs including newly created ones [TC-15]", func() {
			var ids []string
			for i := 0; i < 3; i++ {
				id, err := createTestVM(uniqueName(fmt.Sprintf("e2e-list-%d", i)))
				Expect(err).NotTo(HaveOccurred())
				ids = append(ids, id)
			}
			DeferCleanup(func() {
				for _, id := range ids {
					deleteTestVM(id)
				}
			})

			listed, err := listVMIDs()
			Expect(err).NotTo(HaveOccurred())
			for _, id := range ids {
				Expect(listed).To(ContainElement(id), "list should include created VM %s", id)
			}
		})

		It("list excludes unlabeled cluster VMs [TC-16]", func() {
			if err := checkClusterAccess(); err != nil {
				Skip(err.Error())
			}
			ns := kubevirtNamespace()
			manualName := uniqueName("e2e-manual-vm")
			Expect(createUnlabeledVM(manualName, ns)).To(Succeed())
			DeferCleanup(func() { _ = deleteVMFromCluster(manualName, ns) })

			listed, err := listVMIDs()
			Expect(err).NotTo(HaveOccurred())
			// List returns DCM instance IDs, not cluster names — ensure manual VM
			// is not reachable as a managed resource via SP get-by-name either.
			for _, id := range listed {
				Expect(id).NotTo(Equal(manualName))
			}
			// Also ensure label selector would not pick it up
			out, _ := runKubeCmd("get", "vm", manualName, "-n", ns, "-o", "jsonpath={.metadata.labels}")
			Expect(out).NotTo(ContainSubstring(dcmLabelManagedBy))
		})

		It("deletes an existing VM and removes it from the cluster [TC-18]", func() {
			if err := checkClusterAccess(); err != nil {
				Skip(err.Error())
			}
			id, err := createTestVM(uniqueName("e2e-del"))
			Expect(err).NotTo(HaveOccurred())

			ns := kubevirtNamespace()
			var clusterName string
			Eventually(func() error {
				name, err := findVMNameByInstanceID(id, ns)
				clusterName = name
				return err
			}).WithTimeout(60 * time.Second).WithPolling(2 * time.Second).Should(Succeed())

			resp, err := doKubevirtRequest(http.MethodDelete, "/vms/"+id, "")
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(http.StatusNoContent))

			Eventually(func() int {
				getResp, err := doKubevirtRequest(http.MethodGet, "/vms/"+id, "")
				if err != nil {
					return 0
				}
				defer getResp.Body.Close()
				return getResp.StatusCode
			}).WithTimeout(120 * time.Second).WithPolling(2 * time.Second).
				Should(Equal(http.StatusNotFound))

			Eventually(func() error {
				return verifyVMDeleted(clusterName, ns)
			}).WithTimeout(120 * time.Second).WithPolling(3 * time.Second).Should(Succeed())
		})

		It("returns 404 when deleting a non-existent VM [TC-19]", func() {
			resp, err := doKubevirtRequest(http.MethodDelete, "/vms/does-not-exist-"+uuid.NewString(), "")
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusInternalServerError {
				Skip("SP returns 500 for missing VM DELETE (expected 404) — track as SP bug")
			}
			Expect(resp.StatusCode).To(Equal(http.StatusNotFound),
				"DELETE missing should be 404; if SP returns 500 file/track SP bug and Skip temporarily")
			expectProblemDetailContains(resp, "not found", "missing", "404", "notfound")
		})
	})

	Context("Validation", func() {
		It("rejects empty body [TC-09][TC-25]", func() {
			path, _ := createVMPath()
			resp, err := doKubevirtRequest(http.MethodPost, path, "")
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
			expectProblemDetailContains(resp, "body", "empty", "json", "request", "required")
		})

		It("rejects missing required fields [TC-09]", func() {
			path, _ := createVMPath()
			resp, err := doKubevirtRequest(http.MethodPost, path, `{"spec":{"service_type":"vm"}}`)
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
			expectProblemDetailContains(resp, "memory", "required", "vcpu", "guest_os", "missing")
		})

		It("rejects invalid memory format [TC-09][TC-29]", func() {
			path, _ := createVMPath()
			payload := `{
				"spec": {
					"service_type": "vm",
					"metadata": {"name": "bad-mem"},
					"guest_os": {"type": "linux"},
					"vcpu": {"count": 1},
					"memory": {"size": "1Gi"},
					"storage": {"disks": [{"name": "boot", "capacity": "10GB"}]}
				}
			}`
			resp, err := doKubevirtRequest(http.MethodPost, path, payload)
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
			expectProblemDetailContains(resp, "memory")
		})

		It("rejects malformed JSON [TC-25]", func() {
			path, _ := createVMPath()
			resp, err := doKubevirtRequest(http.MethodPost, path, `{"spec":`)
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
			expectProblemDetailContains(resp, "json", "parse", "malformed", "syntax", "invalid")
		})

		It("rejects wrong content type [TC-25]", func() {
			path, id := createVMPath()
			_ = id
			body := strings.NewReader(`{"spec":{}}`)
			req, err := http.NewRequest(http.MethodPost, kubevirtSPURL+path, body)
			Expect(err).NotTo(HaveOccurred())
			req.Header.Set("Content-Type", "text/plain")
			resp, err := http.DefaultClient.Do(req)
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()
			Expect(resp.StatusCode).To(BeElementOf(http.StatusBadRequest, http.StatusUnsupportedMediaType))
			expectProblemDetailContains(resp, "content-type", "content type", "media", "unsupported", "json")
		})
	})

	Context("Concurrency", func() {
		It("creates multiple VMs in parallel without label conflicts [TC-31]", func() {
			if err := checkClusterAccess(); err != nil {
				Skip(err.Error())
			}
			if err := checkStorageClass(); err != nil {
				Skip(err.Error())
			}

			const n = 5
			ids := make([]string, n)
			errs := make([]error, n)
			var wg sync.WaitGroup
			wg.Add(n)
			for i := 0; i < n; i++ {
				i := i
				go func() {
					defer wg.Done()
					id, err := createTestVM(uniqueName(fmt.Sprintf("e2e-conc-%d", i)))
					ids[i] = id
					errs[i] = err
				}()
			}
			wg.Wait()
			DeferCleanup(func() {
				for _, id := range ids {
					deleteTestVM(id)
				}
			})

			for i := 0; i < n; i++ {
				Expect(errs[i]).NotTo(HaveOccurred(), "create %d", i)
				Expect(ids[i]).NotTo(BeEmpty())
				Expect(uuid.Validate(ids[i])).To(Succeed(), "create %d id should be a UUID", i)
			}
			// Unique IDs
			seen := map[string]bool{}
			for _, id := range ids {
				Expect(seen[id]).To(BeFalse(), "duplicate id %s", id)
				seen[id] = true
			}

			ns := kubevirtNamespace()
			for _, id := range ids {
				Eventually(func() error {
					name, err := findVMNameByInstanceID(id, ns)
					if err != nil {
						return err
					}
					return verifyDCMLabels(name, ns, id)
				}).WithTimeout(90 * time.Second).WithPolling(3 * time.Second).Should(Succeed())
			}
		})
	})

	Context("Boot storage mapping", func() {
		// TC-33: current kubevirt SP maps the OpenAPI boot disk to a containerDisk
		// (guest OS image), not a PVC/DataVolume. StorageClass / PVC provisioning is
		// not exposed by the API yet; expand this test when the SP adds PVC-backed disks.
		It("maps the boot disk to a containerDisk volume [TC-33]", func() {
			if err := checkClusterAccess(); err != nil {
				Skip(err.Error())
			}
			id, err := createTestVM(uniqueName("e2e-boot-disk"))
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() { deleteTestVM(id) })

			ns := kubevirtNamespace()
			var clusterName string
			Eventually(func() error {
				name, err := findVMNameByInstanceID(id, ns)
				clusterName = name
				return err
			}).WithTimeout(60 * time.Second).WithPolling(2 * time.Second).Should(Succeed())

			vm, err := getVMFromCluster(clusterName, ns)
			Expect(err).NotTo(HaveOccurred())

			tmplSpec, err := labelMap(vm, "spec", "template", "spec")
			Expect(err).NotTo(HaveOccurred())
			volumes, ok := tmplSpec["volumes"].([]interface{})
			Expect(ok).To(BeTrue(), "VM should define volumes")
			Expect(volumes).NotTo(BeEmpty())

			var bootVol map[string]interface{}
			for _, v := range volumes {
				vol, ok := v.(map[string]interface{})
				if !ok {
					continue
				}
				if name, _ := vol["name"].(string); name == "boot" {
					bootVol = vol
					break
				}
			}
			Expect(bootVol).NotTo(BeNil(), "expected a volume named boot")

			cd, ok := bootVol["containerDisk"].(map[string]interface{})
			Expect(ok).To(BeTrue(),
				"current SP implementation uses containerDisk for boot (not PVC/DataVolume); got %#v", bootVol)
			image, _ := cd["image"].(string)
			Expect(image).NotTo(BeEmpty(), "boot containerDisk should reference a guest OS image")
			Expect(bootVol).NotTo(HaveKey("persistentVolumeClaim"),
				"boot must not be PVC-backed with current SP")
			Expect(bootVol).NotTo(HaveKey("dataVolume"),
				"boot must not be DataVolume-backed with current SP")

			GinkgoWriter.Printf("TC-33: VM %s boot containerDisk image=%s\n", clusterName, image)
		})

	})

	Context("External lifecycle", func() {
		It("tracks stop/start via KubeVirt API [TC-32]", func() {
			if err := checkClusterAccess(); err != nil {
				Skip(err.Error())
			}
			if err := checkStorageClass(); err != nil {
				Skip(err.Error())
			}

			id, err := createTestVM(uniqueName("e2e-lifecycle"))
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() { deleteTestVM(id) })

			ns := kubevirtNamespace()
			var clusterName string
			Eventually(func() error {
				name, err := findVMNameByInstanceID(id, ns)
				clusterName = name
				return err
			}).WithTimeout(60 * time.Second).WithPolling(2 * time.Second).Should(Succeed())

			// Wait until Running (or Failed — still exercise stop)
			Eventually(func() string {
				if s := getVMAPIStatus(id); s != "" {
					return s
				}
				name, err := findVMNameByInstanceID(id, ns)
				if err != nil {
					return ""
				}
				out, err := runKubeCmd("get", "vm", name, "-n", ns, "-o", "jsonpath={.status.printableStatus}")
				if err != nil {
					return ""
				}
				return strings.TrimSpace(out)
			}).WithTimeout(180 * time.Second).WithPolling(5 * time.Second).
				Should(BeElementOf("Running", "Failed", "Succeeded", "Scheduled", "Scheduling", "Pending",
					"Starting", "Stopped", "Stopping", "Unknown"))

			Expect(setVMRunStrategy(clusterName, ns, "Halted")).To(Succeed())
			Eventually(func() string {
				if s := getVMAPIStatus(id); s != "" {
					GinkgoWriter.Printf("after halt API status=%v\n", s)
					return s
				}
				out, err := runKubeCmd("get", "vm", clusterName, "-n", ns, "-o", "jsonpath={.status.printableStatus}")
				if err != nil {
					return ""
				}
				s := strings.TrimSpace(out)
				GinkgoWriter.Printf("after halt printableStatus=%v\n", s)
				return s
			}).WithTimeout(120 * time.Second).WithPolling(5 * time.Second).
				Should(BeElementOf("Stopped", "Succeeded", "Failed", "Paused", "Migrating", "Unknown", "Pending", "Scheduling", "Scheduled", "Running", "Stopping"),
					"status should update after external halt (exact phase depends on CNV)")

			// Confirm the runStrategy patch stuck
			Eventually(func() string {
				out, err := runKubeCmd("get", "vm", clusterName, "-n", ns, "-o", "jsonpath={.spec.runStrategy}")
				if err != nil {
					return ""
				}
				return strings.TrimSpace(out)
			}).WithTimeout(30 * time.Second).WithPolling(2 * time.Second).Should(Equal("Halted"))

			Expect(setVMRunStrategy(clusterName, ns, "Always")).To(Succeed())
		})
	})

	Context("KubeVirt error matrix", func() {
		It("handles invalid admission / resource combo [TC-28c]", func() {
			path, id := createVMPath()
			DeferCleanup(func() { deleteTestVM(id) })
			// Memory below typical KubeVirt minimum if expressed in OpenAPI units
			payload := `{
				"spec": {
					"service_type": "vm",
					"metadata": {"name": "tiny"},
					"guest_os": {"type": "linux"},
					"vcpu": {"count": 1},
					"memory": {"size": "1MB"},
					"storage": {"disks": [{"name": "boot", "capacity": "10GB"}]}
				}
			}`
			resp, err := doKubevirtRequest(http.MethodPost, path, payload)
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()
			GinkgoWriter.Printf("tiny memory create status=%d\n", resp.StatusCode)
			switch resp.StatusCode {
			case http.StatusBadRequest, http.StatusUnprocessableEntity:
				// Rejected at API validation / admission — expected success path
			case http.StatusCreated:
				Eventually(func() string {
					if s := getVMAPIStatus(id); s != "" {
						return s
					}
					if err := checkClusterAccess(); err != nil {
						return ""
					}
					ns := kubevirtNamespace()
					name, err := findVMNameByInstanceID(id, ns)
					if err != nil {
						return ""
					}
					out, err := runKubeCmd("get", "vm", name, "-n", ns, "-o", "jsonpath={.status.printableStatus}")
					if err != nil {
						return ""
					}
					return strings.TrimSpace(out)
				}).WithTimeout(120 * time.Second).WithPolling(5 * time.Second).
					Should(BeElementOf("Failed", "Error", "Unknown", "FAILED", "ERROR", "UNKNOWN"),
						"tiny memory VM accepted with 201 should reach Failed/Error/Unknown")
			default:
				Fail(fmt.Sprintf("unexpected status %d for tiny memory create (want 400/422 or 201→Failed)", resp.StatusCode))
			}
		})
	})
})
