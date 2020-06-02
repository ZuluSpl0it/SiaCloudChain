package host

import (
	"container/heap"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"gitlab.com/scpcorp/ScPrime/build"
	"gitlab.com/scpcorp/ScPrime/modules"
	"gitlab.com/scpcorp/ScPrime/siatest/dependencies"
	"gitlab.com/scpcorp/ScPrime/types"

	"gitlab.com/NebulousLabs/errors"
	"gitlab.com/NebulousLabs/fastrand"
)

// TestPriceTableMarshaling tests a PriceTable can be marshaled and unmarshaled
func TestPriceTableMarshaling(t *testing.T) {
	pt := modules.RPCPriceTable{
		Expiry:               time.Now().Add(rpcPriceGuaranteePeriod).Unix(),
		HostBlockHeight:      types.BlockHeight(fastrand.Intn(1e3)),
		UpdatePriceTableCost: types.SiacoinPrecision,
		InitBaseCost:         types.SiacoinPrecision.Mul64(1e2),
		MemoryTimeCost:       types.SiacoinPrecision.Mul64(1e3),
		ReadBaseCost:         types.SiacoinPrecision.Mul64(1e4),
		ReadLengthCost:       types.SiacoinPrecision.Mul64(1e5),
		HasSectorBaseCost:    types.SiacoinPrecision.Mul64(1e6),
	}
	fastrand.Read(pt.UID[:])

	ptBytes, err := json.Marshal(pt)
	if err != nil {
		t.Fatal(err)
	}
	var ptCopy modules.RPCPriceTable
	err = json.Unmarshal(ptBytes, &ptCopy)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(pt, ptCopy) {
		t.Log(pt.UID[:])
		t.Log(ptCopy.UID[:])
		t.Fatal(errors.New("PriceTable not equal after unmarshaling"))
	}
}

// TestPriceTableMinHeap verifies the working of the min heap
func TestPriceTableMinHeap(t *testing.T) {
	t.Parallel()

	now := time.Now()
	pth := priceTableHeap{heap: make([]*modules.RPCPriceTable, 0)}

	// add 4 price tables (out of order) that expire somewhere in the future
	pt1 := modules.RPCPriceTable{Expiry: now.Add(9 * time.Minute).Unix()}
	pt2 := modules.RPCPriceTable{Expiry: now.Add(-3 * time.Minute).Unix()}
	pt3 := modules.RPCPriceTable{Expiry: now.Add(-6 * time.Minute).Unix()}
	pt4 := modules.RPCPriceTable{Expiry: now.Add(-1 * time.Minute).Unix()}
	pth.Push(&pt1)
	pth.Push(&pt2)
	pth.Push(&pt3)
	pth.Push(&pt4)

	// verify it considers 3 to be expired if we pass it a threshold 7' from now
	expired := pth.PopExpired()
	if len(expired) != 3 {
		t.Fatalf("Expected 3 price tables to be expired, yet managedExpired returned %d price tables", len(expired))
	}

	// verify 'pop' returns the last remaining price table
	pth.mu.Lock()
	expectedPt1 := heap.Pop(&pth.heap)
	pth.mu.Unlock()
	if expectedPt1 != &pt1 {
		t.Fatal("Expected the last price table to be equal to pt1, which is the price table with the highest expiry")
	}
}

// TestPruneExpiredPriceTables verifies the rpc price tables get pruned from the
// host's price table map if they have expired.
func TestPruneExpiredPriceTables(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()

	// create a blank host tester
	rhp, err := newRenterHostPair(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		err := rhp.Close()
		if err != nil {
			t.Error(err)
		}
	}()
	ht := rhp.staticHT

	// verify the price table is being tracked
	pt, err := rhp.managedFetchPriceTable()
	if err != nil {
		t.Fatal(err)
	}

	_, tracked := ht.host.staticPriceTables.managedGet(pt.UID)
	if !tracked {
		t.Fatal("Expected the testing price table to be tracked but isn't")
	}

	// sleep for the duration of the expiry frequency, seeing as that is greater
	// than the price guarantee period, it is the worst case
	err = build.Retry(10, pruneExpiredRPCPriceTableFrequency, func() error {
		_, exists := ht.host.staticPriceTables.managedGet(pt.UID)
		if exists {
			return errors.New("Expected RPC price table to be pruned because it should have expired")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestUpdatePriceTableRPC tests the UpdatePriceTableRPC by manually calling the
// RPC handler.
func TestUpdatePriceTableRPC(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()

	// setup a host and renter pair with an emulated file contract between them
	pair, err := newRenterHostPair(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	ht := pair.staticHT

	// renter-side logic
	runWithRequest := func(pbcRequest modules.PayByContractRequest) (*modules.RPCPriceTable, error) {
		stream := pair.managedNewStream()
		defer stream.Close()

		// initiate the RPC
		err = modules.RPCWrite(stream, modules.RPCUpdatePriceTable)
		if err != nil {
			return nil, err
		}

		// receive the price table response
		var pt modules.RPCPriceTable
		var update modules.RPCUpdatePriceTableResponse
		err = modules.RPCRead(stream, &update)
		if err != nil {
			return nil, err
		}
		if err = json.Unmarshal(update.PriceTableJSON, &pt); err != nil {
			return nil, err
		}

		// send PaymentRequest & PayByContractRequest
		pRequest := modules.PaymentRequest{Type: modules.PayByContract}
		err = modules.RPCWriteAll(stream, pRequest, pbcRequest)
		if err != nil {
			return nil, err
		}

		// receive PayByContractResponse
		var payByResponse modules.PayByContractResponse
		err = modules.RPCRead(stream, &payByResponse)
		if err != nil {
			return nil, err
		}

		// await tracked response
		var tracked modules.RPCTrackedPriceTableResponse
		err = modules.RPCRead(stream, &tracked)
		if err != nil {
			return nil, err
		}
		return &pt, nil
	}

	// create an account id
	var aid modules.AccountID
	err = aid.LoadString("prefix:deadbeef")
	if err != nil {
		t.Fatal(err)
	}

	// verify happy flow
	current := ht.host.staticPriceTables.managedCurrent()
	rev, sig, err := pair.managedPaymentRevision(current.UpdatePriceTableCost)
	if err != nil {
		t.Fatal(err)
	}

	pt, err := runWithRequest(newPayByContractRequest(rev, sig, aid))
	if err != nil {
		t.Fatal(err)
	}
	// ensure the price table is tracked by the host
	_, tracked := ht.host.staticPriceTables.managedGet(pt.UID)
	if !tracked {
		t.Fatalf("Expected price table with.UID %v to be tracked after successful update", pt.UID)
	}
	// ensure its expiry is in the future
	if pt.Expiry <= time.Now().Unix() {
		t.Fatal("Expected price table expiry to be in the future")
	}

	// ensure it contains the host's block height
	if pt.HostBlockHeight != ht.host.BlockHeight() {
		t.Fatal("Expected host blockheight to be set on the price table")
	}
	// ensure it's not zero to be certain the blockheight is set and it's not
	// just the initial value
	if pt.HostBlockHeight == 0 {
		t.Fatal("Expected host blockheight to be not 0")
	}

	// expect failure if the payment revision does not cover the RPC cost
	rev, sig, err = pair.managedPaymentRevision(types.ZeroCurrency)
	if err != nil {
		t.Fatal(err)
	}
	_, err = runWithRequest(newPayByContractRequest(rev, sig, aid))
	if err == nil || !strings.Contains(err.Error(), modules.ErrInsufficientPaymentForRPC.Error()) {
		t.Fatalf("Expected error '%v', instead error was '%v'", modules.ErrInsufficientPaymentForRPC, err)
	}

	// close the pair and recreate one with a custom dependency
	err = pair.Close()
	if err != nil {
		t.Error(err)
	}

	// create a new renter host pair but now with a dependency that prevents the
	// stream from closing
	deps := &dependencies.DependencyDisableStreamClose{}
	pair, err = newCustomRenterHostPair(t.Name(), deps)
	defer func() {
		err := pair.Close()
		if err != nil {
			t.Error(err)
		}
	}()
	ht = pair.staticHT

	// verify the RPC does not block if the host does not close the stream on
	// his side
	current = ht.host.staticPriceTables.managedCurrent()
	rev, sig, err = pair.managedPaymentRevision(current.UpdatePriceTableCost)
	if err != nil {
		t.Fatal(err)
	}

	_, err = runWithRequest(newPayByContractRequest(rev, sig, aid))
	if err != nil {
		t.Fatal(err)
	}
}
