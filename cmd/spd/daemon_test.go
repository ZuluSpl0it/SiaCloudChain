package main

import (
	"testing"
)

// TestUnitProcessNetAddr probes the 'processNetAddr' function.
func TestUnitProcessNetAddr(t *testing.T) {
	testVals := struct {
		inputs          []string
		expectedOutputs []string
	}{
		inputs:          []string{"9510", ":9510", "localhost:9510", "test.com:9510", "192.168.14.92:9510"},
		expectedOutputs: []string{":9510", ":9510", "localhost:9510", "test.com:9510", "192.168.14.92:9510"},
	}
	for i, input := range testVals.inputs {
		output := processNetAddr(input)
		if output != testVals.expectedOutputs[i] {
			t.Error("unexpected result", i)
		}
	}
}

// TestUnitProcessModules tests that processModules correctly processes modules
// passed to the -M / --modules flag.
func TestUnitProcessModules(t *testing.T) {
	// Test valid modules.
	testVals := []struct {
		in  string
		out string
	}{
		{"cghmrtwe", "cghmrtwe"},
		{"CGHMRTWE", "cghmrtwe"},
		{"c", "c"},
		{"g", "g"},
		{"h", "h"},
		{"m", "m"},
		{"r", "r"},
		{"t", "t"},
		{"w", "w"},
		{"e", "e"},
		{"C", "c"},
		{"G", "g"},
		{"H", "h"},
		{"M", "m"},
		{"R", "r"},
		{"T", "t"},
		{"W", "w"},
		{"E", "e"},
	}
	for _, testVal := range testVals {
		out, err := processModules(testVal.in)
		if err != nil {
			t.Error("processModules failed with error:", err)
		}
		if out != testVal.out {
			t.Errorf("processModules returned incorrect modules: expected %s, got %s\n", testVal.out, out)
		}
	}

	// Test invalid modules.
	invalidModules := []string{"abdfijklnopqsuvxyz", "cghmrtwez", "cz", "z", "cc", "ccz", "ccm", "cmm", "ccmm"}
	for _, invalidModule := range invalidModules {
		_, err := processModules(invalidModule)
		if err == nil {
			t.Error("processModules didn't error on invalid module:", invalidModule)
		}
	}
}

// TestUnitProcessProfile tests that processProfileFlags correctly processes profiles
// passed to the --profile flag.
func TestUnitProcessProfile(t *testing.T) {
	// Test valid profiles.
	testVals := []struct {
		in  string
		out string
	}{
		{"cmt", "cmt"},
		{"CMT", "cmt"},
		{"c", "c"},
		{"m", "m"},
		{"t", "t"},
		{"C", "c"},
		{"M", "m"},
		{"T", "t"},
	}
	for _, testVal := range testVals {
		out, err := processProfileFlags(testVal.in)
		if err != nil {
			t.Error("processProfileFlags failed with error:", err)
		}
		if out != testVal.out {
			t.Errorf("processProfileFlags returned incorrect modules: expected %s, got %s\n", testVal.out, out)
		}
	}

	// Test invalid modules.
	invalidProfiles := []string{"abdfijklnopqsuvxyz", "cghmrtwez", "cz", "z", "cc", "ccz", "ccm", "cmm", "ccmm", "g", "h", "cghmrtwe", "CGHMRTWE", "mts"}
	for _, invalidProfiles := range invalidProfiles {
		_, err := processProfileFlags(invalidProfiles)
		if err == nil {
			t.Error("processProfileFlags didn't error on invalid profile:", invalidProfiles)
		}
	}
}

// TestUnitProcessConfig probes the 'processConfig' function.
func TestUnitProcessConfig(t *testing.T) {
	// Test valid configs.
	testVals := struct {
		inputs          [][]string
		expectedOutputs [][]string
	}{
		inputs: [][]string{
			{"localhost:9510", "localhost:9511", "localhost:9514", "cghmrtwe"},
			{"localhost:9510", "localhost:9511", "localhost:9514", "CGHMRTWE"},
		},
		expectedOutputs: [][]string{
			{"localhost:9510", "localhost:9511", "localhost:9514", "cghmrtwe"},
			{"localhost:9510", "localhost:9511", "localhost:9514", "cghmrtwe"},
		},
	}
	var config Config
	for i := range testVals.inputs {
		config.Spd.APIaddr = testVals.inputs[i][0]
		config.Spd.RPCaddr = testVals.inputs[i][1]
		config.Spd.HostAddr = testVals.inputs[i][2]
		config, err := processConfig(config)
		if err != nil {
			t.Error("processConfig failed with error:", err)
		}
		if config.Spd.APIaddr != testVals.expectedOutputs[i][0] {
			t.Error("processing failure at check", i, 0)
		}
		if config.Spd.RPCaddr != testVals.expectedOutputs[i][1] {
			t.Error("processing failure at check", i, 1)
		}
		if config.Spd.HostAddr != testVals.expectedOutputs[i][2] {
			t.Error("processing failure at check", i, 2)
		}
	}

	// Test invalid configs.
	invalidModule := "z"
	config.Spd.Modules = invalidModule
	_, err := processConfig(config)
	if err == nil {
		t.Error("processModules didn't error on invalid module:", invalidModule)
	}
}

// TestLoadAPIPassword tests the 'loadAPIPassword' function.
func TestLoadAPIPassword(t *testing.T) {
	// If config.Siad.AuthenticateAPI is false, no password should be set
	var config Config

	config, err := loadAPIPassword(config)
	if err != nil {
		t.Fatal(err)
	} else if config.APIPassword != "" {
		t.Fatal("loadAPIPassword should not set a password if config.Spd.AuthenticateAPI is false")
	}
	config.Spd.AuthenticateAPI = true
	// On first invocation, loadAPIPassword should generate a new random
	// password
	config2, err := loadAPIPassword(config)
	if err != nil {
		t.Fatal(err)
	} else if config2.APIPassword == "" {
		t.Fatal("loadAPIPassword should have generated a random password")
	}
	// On subsequent invocations, loadAPIPassword should use the
	// previously-generated password
	config3, err := loadAPIPassword(config)
	if err != nil {
		t.Fatal(err)
	} else if config3.APIPassword != config2.APIPassword {
		t.Fatal("loadAPIPassword should have used previously-generated password")
	}
}

// TestVerifyAPISecurity checks that the verifyAPISecurity function is
// correctly banning the use of a non-loopback address without the
// --disable-security flag, and that the --disable-security flag cannot be used
// without an api password.
func TestVerifyAPISecurity(t *testing.T) {
	// Check that the loopback address is accepted when security is enabled.
	var securityOnLoopback Config
	securityOnLoopback.Spd.APIaddr = "127.0.0.1:9510"
	err := verifyAPISecurity(securityOnLoopback)
	if err != nil {
		t.Error("loopback + securityOn was rejected")
	}

	// Check that the blank address is rejected when security is enabled.
	var securityOnBlank Config
	securityOnBlank.Spd.APIaddr = ":9510"
	err = verifyAPISecurity(securityOnBlank)
	if err == nil {
		t.Error("blank + securityOn was accepted")
	}

	// Check that a public hostname is rejected when security is enabled.
	var securityOnPublic Config
	securityOnPublic.Spd.APIaddr = "scpri.me:9510"
	err = verifyAPISecurity(securityOnPublic)
	if err == nil {
		t.Error("public + securityOn was accepted")
	}

	// Check that a public hostname is rejected when security is disabled and
	// there is no api password.
	var securityOffPublic Config
	securityOffPublic.Spd.APIaddr = "scpri.me:9510"
	securityOffPublic.Spd.AllowAPIBind = true
	err = verifyAPISecurity(securityOffPublic)
	if err == nil {
		t.Error("public + securityOff was accepted without authentication")
	}

	// Check that a public hostname is accepted when security is disabled and
	// there is an api password.
	var securityOffPublicAuthenticated Config
	securityOffPublicAuthenticated.Spd.APIaddr = "scpri.me:9510"
	securityOffPublicAuthenticated.Spd.AllowAPIBind = true
	securityOffPublicAuthenticated.Spd.AuthenticateAPI = true
	err = verifyAPISecurity(securityOffPublicAuthenticated)
	if err != nil {
		t.Error("public + securityOff with authentication was rejected:", err)
	}
}
