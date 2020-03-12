package transactionpool

import (
	"os"

	"gitlab.com/scpcorp/ScPrime/persist"
	"gitlab.com/scpcorp/ScPrime/siatest"
)

// tpoolTestDir creates a temporary testing directory for a transaction pool
// test. This should only every be called once per test. Otherwise it will
// delete the directory again.
func tpoolTestDir(testName string) string {
	path := siatest.TestDir("transactionpool", testName)
	if err := os.MkdirAll(path, persist.DefaultDiskPermissionsTest); err != nil {
		panic(err)
	}
	return path
}
