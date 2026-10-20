//go:build sanity

// Package sanitytest runs the CSI sanity suite against a real DSM: make sanity.
package sanitytest

import (
	"fmt"
	"os"
	"testing"

	sanity "github.com/kubernetes-csi/csi-test/v5/pkg/sanity"

	"github.com/inistor/synology-csi/pkg/driver"
	"github.com/inistor/synology-csi/pkg/dsm/common"
	"github.com/inistor/synology-csi/pkg/dsm/service"
	"github.com/inistor/synology-csi/pkg/utils/hostexec"
)

const (
	ConfigPath      = "./../../config/client-info.yml"
	SecretsFilePath = "./sanity-test-secret-file.yaml"
)

func TestSanity(t *testing.T) {
	nodeID := "CSINode"

	tmp := t.TempDir()
	endpointPath := tmp + "/csi.sock"
	// The suite creates these itself.
	stagingPath := tmp + "/staging"
	targetPath := tmp + "/target"

	info, err := common.LoadConfig(ConfigPath)
	if err != nil {
		t.Fatal(fmt.Sprintf("Failed to read config: %v", err))
	}

	dsmService := service.NewDsmService()

	for _, client := range info.Clients {
		err := dsmService.AddDsm(client)
		if err != nil {
			fmt.Printf("Failed to add DSM: %s, error: %v\n", client.Host, err)
		}
	}

	if dsmService.GetDsmsCount() == 0 {
		t.Fatal("No available DSM.")
	}
	defer dsmService.RemoveAllDsms()

	cmdExecutor, err := hostexec.New(nil, "")
	if err != nil {
		t.Fatal(fmt.Sprintf("Failed to create command executor: %v\n", err))
	}
	tools := driver.NewTools(cmdExecutor)

	endpoint := "unix://" + endpointPath
	drv, err := driver.NewControllerAndNodeDriver(driver.DriverName, nodeID, endpoint, dsmService, tools)
	if err != nil {
		t.Fatal(fmt.Sprintf("Failed to create driver: %v\n", err))
	}
	drv.Activate()

	// Set configuration options as needed
	testConfig := sanity.NewTestConfig()
	testConfig.TargetPath = targetPath
	testConfig.StagingPath = stagingPath
	testConfig.Address = endpoint
	testConfig.SecretsFile = SecretsFilePath

	// Set Input parameters for test
	// SANITY_PROTOCOL picks the protocol under test (default smb).
	protocol := os.Getenv("SANITY_PROTOCOL")
	if protocol == "" {
		protocol = "smb"
	}
	testConfig.TestVolumeParameters = map[string]string{
		"protocol": protocol,
	}

	// testConfig.TestVolumeAccessType = "block" // raw block

	// Run test
	sanity.Test(t, testConfig)
}
