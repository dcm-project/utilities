//go:build e2e

package e2e_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("CLI: version command", Label("smoke", "cli"), func() {
	It("prints version information", func() {
		stdout, _, exitCode := runDCM("version")

		Expect(exitCode).To(Equal(0))
		Expect(stdout).To(ContainSubstring("dcm version"))
	})
})
