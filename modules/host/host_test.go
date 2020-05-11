package host

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"gitlab.com/NebulousLabs/errors"
	"gitlab.com/NebulousLabs/fastrand"

	"gitlab.com/scpcorp/ScPrime/build"
	"gitlab.com/scpcorp/ScPrime/crypto"
	"gitlab.com/scpcorp/ScPrime/modules"
	"gitlab.com/scpcorp/ScPrime/modules/consensus"
	"gitlab.com/scpcorp/ScPrime/modules/gateway"
	"gitlab.com/scpcorp/ScPrime/modules/miner"
	"gitlab.com/scpcorp/ScPrime/modules/transactionpool"
	"gitlab.com/scpcorp/ScPrime/modules/wallet"
	"gitlab.com/scpcorp/ScPrime/persist"
	siasync "gitlab.com/scpcorp/ScPrime/sync"
	"gitlab.com/scpcorp/ScPrime/types"

	"gitlab.com/scpcorp/siamux"
	"gitlab.com/scpcorp/siamux/mux"
)

// A hostTester is the helper object for host testing, including helper modules
// and methods for controlling synchronization.
type (
	closeFn func() error

	hostTester struct {
		mux *siamux.SiaMux

		cs        modules.ConsensusSet
		gateway   modules.Gateway
		miner     modules.TestMiner
		tpool     modules.TransactionPool
		wallet    modules.Wallet
		walletKey crypto.CipherKey

		host *Host

		persistDir string
	}
)

/*
// initRenting prepares the host tester for uploads and downloads by announcing
// the host to the network and performing other preparational tasks.
// initRenting takes a while because the renter needs to process the host
// announcement, requiring asynchronous network communication between the
// renter and host.
func (ht *hostTester) initRenting() error {
	if ht.renting {
		return nil
	}

	// Because the renting test takes a long time, it will fail if
	// testing.Short.
	if testing.Short() {
		return errors.New("cannot call initRenting in short tests")
	}

	// Announce the host.
	err := ht.host.Announce()
	if err != nil {
		return err
	}

	// Mine a block to get the announcement into the blockchain.
	_, err = ht.miner.AddBlock()
	if err != nil {
		return err
	}

	// Wait for the renter to see the host announcement.
	for i := 0; i < 50; i++ {
		time.Sleep(time.Millisecond * 100)
		if len(ht.renter.ActiveHosts()) != 0 {
			break
		}
	}
	if len(ht.renter.ActiveHosts()) == 0 {
		return errors.New("could not start renting in the host tester")
	}
	ht.renting = true
	return nil
}
*/

// initWallet creates a wallet key, initializes the host wallet, unlocks it,
// and then stores the key in the host tester.
func (ht *hostTester) initWallet() error {
	// Create the keys for the wallet and unlock it.
	key := crypto.GenerateSiaKey(crypto.TypeDefaultWallet)
	ht.walletKey = key
	_, err := ht.wallet.Encrypt(key)
	if err != nil {
		return err
	}
	err = ht.wallet.Unlock(key)
	if err != nil {
		return err
	}
	return nil
}

// blankHostTester creates a host tester where the modules are created but no
// extra initialization has been done, for example no blocks have been mined
// and the wallet keys have not been created.
func blankHostTester(name string) (*hostTester, error) {
	return blankMockHostTester(modules.ProdDependencies, name)
}

// blankMockHostTester creates a host tester where the modules are created but no
// extra initialization has been done, for example no blocks have been mined
// and the wallet keys have not been created.
func blankMockHostTester(d modules.Dependencies, name string) (*hostTester, error) {
	testdir := build.TempDir(modules.HostDir, name)

	// Create the siamux.
	siaMuxDir := filepath.Join(testdir, modules.SiaMuxDir)
	mux, err := modules.NewSiaMux(siaMuxDir, testdir, "localhost:0")
	if err != nil {
		return nil, err
	}

	// Create the modules.
	g, err := gateway.New("localhost:0", false, filepath.Join(testdir, modules.GatewayDir))
	if err != nil {
		return nil, err
	}
	cs, errChan := consensus.New(g, false, filepath.Join(testdir, modules.ConsensusDir))
	if err := <-errChan; err != nil {
		return nil, err
	}
	tp, err := transactionpool.New(cs, g, filepath.Join(testdir, modules.TransactionPoolDir))
	if err != nil {
		return nil, err
	}
	w, err := wallet.New(cs, tp, filepath.Join(testdir, modules.WalletDir))
	if err != nil {
		return nil, err
	}
	m, err := miner.New(cs, tp, w, filepath.Join(testdir, modules.MinerDir))
	if err != nil {
		return nil, err
	}
	h, err := NewCustomHost(d, cs, g, tp, w, mux, "localhost:0", filepath.Join(testdir, modules.HostDir))
	if err != nil {
		return nil, err
	}
	/*
		r, err := renter.New(cs, w, tp, filepath.Join(testdir, modules.RenterDir))
		if err != nil {
			return nil, err
		}
	*/

	// Assemble all objects into a hostTester
	ht := &hostTester{
		mux: mux,

		cs:      cs,
		gateway: g,
		miner:   m,
		// renter:  r,
		tpool:  tp,
		wallet: w,

		host: h,

		persistDir: testdir,
	}

	return ht, nil
}

// newHostTester creates a host tester with an initialized wallet and money in
// that wallet.
func newHostTester(name string) (*hostTester, error) {
	return newMockHostTester(modules.ProdDependencies, name)
}

// newMockHostTester creates a host tester with an initialized wallet and money
// in that wallet, using the dependencies provided.
func newMockHostTester(d modules.Dependencies, name string) (*hostTester, error) {
	// Create a blank host tester.
	ht, err := blankMockHostTester(d, name)
	if err != nil {
		return nil, err
	}

	// Initialize the wallet and mine blocks until the wallet has money.
	err = ht.initWallet()
	if err != nil {
		return nil, err
	}
	for i := types.BlockHeight(0); i <= types.MaturityDelay; i++ {
		_, err = ht.miner.AddBlock()
		if err != nil {
			return nil, err
		}
	}

	// Create two storage folder for the host, one the minimum size and one
	// twice the minimum size.
	storageFolderOne := filepath.Join(ht.persistDir, "hostTesterStorageFolderOne")
	err = os.Mkdir(storageFolderOne, 0700)
	if err != nil {
		return nil, err
	}
	err = ht.host.AddStorageFolder(storageFolderOne, modules.SectorSize*64)
	if err != nil {
		return nil, err
	}
	storageFolderTwo := filepath.Join(ht.persistDir, "hostTesterStorageFolderTwo")
	err = os.Mkdir(storageFolderTwo, 0700)
	if err != nil {
		return nil, err
	}
	err = ht.host.AddStorageFolder(storageFolderTwo, modules.SectorSize*64*2)
	if err != nil {
		return nil, err
	}
	return ht, nil
}

// Close safely closes the hostTester. It panics if err != nil because there
// isn't a good way to errcheck when deferring a close.
func (ht *hostTester) Close() error {
	errs := []error{
		ht.host.Close(),
		ht.miner.Close(),
		ht.wallet.Close(),
		ht.tpool.Close(),
		ht.cs.Close(),
		ht.gateway.Close(),
		ht.mux.Close(),
	}
	if err := build.JoinErrors(errs, "; "); err != nil {
		panic(err)
	}
	return nil
}

// renterHostPair is a helper struct that contains a secret key, symbolizing the
// renter, a host and the id of the file contract they share.
type renterHostPair struct {
	staticAccountID  modules.AccountID
	staticAccountKey crypto.SecretKey
	staticFCID       types.FileContractID
	staticRenterSK   crypto.SecretKey
	staticRenterPK   types.SiaPublicKey
	staticRenterMux  *siamux.SiaMux

	ht *hostTester
	pt *modules.RPCPriceTable
	mu sync.Mutex
}

// newRenterHostPair creates a new host tester and returns a renter host pair,
// this pair is a helper struct that contains both the host and renter,
// represented by its secret key. This helper will create a storage
// obligation emulating a file contract between them.
func newRenterHostPair(name string) (*renterHostPair, error) {
	return newCustomRenterHostPair(name, modules.ProdDependencies)
}

// newCustomRenterHostPair creates a new host tester and returns a renter host
// pair, this pair is a helper struct that contains both the host and renter,
// represented by its secret key. This helper will create a storage obligation
// emulating a file contract between them. It is custom as it allows passing a
// set of dependencies.
func newCustomRenterHostPair(name string, deps modules.Dependencies) (*renterHostPair, error) {
	// setup host
	ht, err := newMockHostTester(deps, name)
	if err != nil {
		return nil, err
	}
	return newRenterHostPairCustomHostTester(ht)
}

// newRenterHostPairCustomHostTester returns a renter host pair, this pair is a
// helper struct that contains both the host and renter, represented by its
// secret key. This helper will create a storage obligation emulating a file
// contract between them. This method requires the caller to pass a hostTester
// opposed to creating one, which allows setting up multiple renters which each
// have a contract with the one host.
func newRenterHostPairCustomHostTester(ht *hostTester) (*renterHostPair, error) {
	// create a renter key pair
	sk, pk := crypto.GenerateKeyPair()
	renterPK := types.SiaPublicKey{
		Algorithm: types.SignatureEd25519,
		Key:       pk[:],
	}

	// setup storage obligation (emulating a renter creating a contract)
	so, err := ht.newTesterStorageObligation()
	if err != nil {
		return nil, errors.AddContext(err, "unable to make the new tester storage obligation")
	}
	so, err = ht.addNoOpRevision(so, renterPK)
	if err != nil {
		return nil, errors.AddContext(err, "unable to add noop revision")
	}
	ht.host.managedLockStorageObligation(so.id())
	err = ht.host.managedAddStorageObligation(so, false)
	if err != nil {
		return nil, errors.AddContext(err, "unable to add the storage obligation")
	}
	ht.host.managedUnlockStorageObligation(so.id())

	// prepare an EA without funding it.
	accountKey, accountID := prepareAccount()

	// prepare a siamux for the renter
	renterMuxDir := filepath.Join(ht.persistDir, "rentermux")
	if err := os.MkdirAll(renterMuxDir, 0700); err != nil {
		return nil, errors.AddContext(err, "unable to mkdirall")
	}
	muxLogger, err := persist.NewFileLogger(filepath.Join(renterMuxDir, "siamux.log"))
	if err != nil {
		return nil, errors.AddContext(err, "unable to create mux logger")
	}
	renterMux, err := siamux.New("127.0.0.1:0", muxLogger, renterMuxDir)
	if err != nil {
		return nil, errors.AddContext(err, "unable to create renter mux")
	}

	pair := &renterHostPair{
		staticAccountID:  accountID,
		staticAccountKey: accountKey,
		staticRenterSK:   sk,
		staticRenterPK:   renterPK,
		staticRenterMux:  renterMux,
		staticFCID:       so.id(),
		ht:               ht,
	}

	// fetch a price table
	err = pair.UpdatePriceTable(true)
	if err != nil {
		return nil, errors.AddContext(err, "unable to update price table")
	}

	// sanity check to verify the refund account used to update the PT is empty
	// to ensure the test starts with a clean slate
	am := pair.ht.host.staticAccountManager
	balance := am.callAccountBalance(pair.staticAccountID)
	if !balance.IsZero() {
		return nil, errors.New("account balance was not zero after initialising a renter host pair")
	}

	return pair, nil
}

// Close closes the underlying host tester.
func (p *renterHostPair) Close() error {
	err1 := p.staticRenterMux.Close()
	err2 := p.ht.Close()
	return errors.Compose(err1, err2)
}

// FetchPriceTable returns the latest price table, if that price table is
// expired it will fetch a new one from the host.
func (p *renterHostPair) FetchPriceTable() (*modules.RPCPriceTable, error) {
	// fetch a new pricetable if it's about to expire, rather than the second it
	// expires. This ensures calls performed immediately after `FetchPriceTable`
	// is called are set to succeed.
	var expiryBuffer int64 = 3

	pt := p.PriceTable()
	if pt.Expiry <= time.Now().Unix()+expiryBuffer {
		err := p.UpdatePriceTable(true)
		if err != nil {
			return nil, err
		}
		return p.PriceTable(), nil
	}
	return pt, nil
}

// PriceTable returns the latest price table
func (p *renterHostPair) PriceTable() *modules.RPCPriceTable {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.pt
}

// SetPriceTable sets the given price table
func (p *renterHostPair) SetPriceTable(pt *modules.RPCPriceTable) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.pt = pt
}

// addRandomSector is a helper function that creates a random sector and adds it
// to the storage obligation.
func (p *renterHostPair) addRandomSector() (crypto.Hash, []byte, error) {
	// create a random sector
	sectorData := fastrand.Bytes(int(modules.SectorSize))
	sectorRoot := crypto.MerkleRoot(sectorData)

	// fetch the SO
	so, err := p.ht.host.managedGetStorageObligation(p.staticFCID)
	if err != nil {
		return crypto.Hash{}, nil, err
	}

	// add a new revision
	so.SectorRoots = append(so.SectorRoots, sectorRoot)
	so, err = p.ht.addNewRevision(so, p.staticRenterPK, uint64(len(sectorData)), sectorRoot)
	if err != nil {
		return crypto.Hash{}, nil, err
	}

	// modify the SO
	p.ht.host.managedLockStorageObligation(p.staticFCID)
	err = p.ht.host.managedModifyStorageObligation(so, []crypto.Hash{}, map[crypto.Hash][]byte{sectorRoot: sectorData})
	if err != nil {
		p.ht.host.managedUnlockStorageObligation(p.staticFCID)
		return crypto.Hash{}, nil, err
	}
	p.ht.host.managedUnlockStorageObligation(p.staticFCID)

	return sectorRoot, sectorData, nil
}

// executeProgramResponse is a helper struct that wraps the
// RPCExecuteProgramResponse together with the output data
type executeProgramResponse struct {
	modules.RPCExecuteProgramResponse
	Output []byte
}

// ExecuteProgram executes an MDM program on the host using an EA payment and
// returns the responses received by the host. A failure to execute an
// instruction won't result in an error. Instead the returned responses need to
// be inspected for that depending on the testcase.
func (p *renterHostPair) ExecuteProgram(epr modules.RPCExecuteProgramRequest, programData []byte, budget types.Currency, updatePriceTable bool) ([]executeProgramResponse, mux.BandwidthLimit, error) {
	var err error

	pt := p.PriceTable()
	if updatePriceTable {
		pt, err = p.FetchPriceTable()
		if err != nil {
			return nil, nil, errors.AddContext(err, "error fetching price table")
		}
	}

	// create stream
	stream := p.newStream()
	defer stream.Close()

	// Get the limit to track bandwidth.
	limit := stream.Limit()

	// Write the specifier.
	err = modules.RPCWrite(stream, modules.RPCExecuteProgram)
	if err != nil {
		return nil, limit, errors.AddContext(err, "RPCWrite RPCExecuteProgram failed")
	}

	// Write the pricetable uid.
	err = modules.RPCWrite(stream, pt.UID)
	if err != nil {
		return nil, limit, errors.AddContext(err, "RPCWrite price table UID failed")
	}

	// Send the payment request.
	err = modules.RPCWrite(stream, modules.PaymentRequest{Type: modules.PayByEphemeralAccount})
	if err != nil {
		return nil, limit, errors.AddContext(err, "RPCWrite error submitting payment request")
	}

	// Send the payment details.
	pbear := newPayByEphemeralAccountRequest(p.staticAccountID, p.ht.host.BlockHeight()+6, budget, p.staticAccountKey)
	err = modules.RPCWrite(stream, pbear)
	if err != nil {
		return nil, limit, errors.AddContext(err, "RPCWrite error submitting PayByEphemeralAccountRequest")
	}

	// Receive payment confirmation.
	var pc modules.PayByEphemeralAccountResponse
	err = modules.RPCRead(stream, &pc)
	if err != nil {
		return nil, limit, errors.AddContext(err, "Error receiving PayByEphemeralAccount response")
	}

	// Send the execute program request.
	err = modules.RPCWrite(stream, epr)
	if err != nil {
		return nil, limit, errors.AddContext(err, "RPCWrite error submitting RPCExecuteProgramRequest")
	}

	// Send the programData.
	_, err = stream.Write(programData)
	if err != nil {
		return nil, limit, errors.AddContext(err, "error writing program data stream")
	}

	// Read the responses.
	responses := make([]executeProgramResponse, len(epr.Program))
	for i := range epr.Program {
		// Read the response.
		err = modules.RPCRead(stream, &responses[i])
		if err != nil {
			return nil, limit, errors.AddContext(err, "error reading response")
		}

		// Read the output data.
		outputLen := responses[i].OutputLength
		responses[i].Output = make([]byte, outputLen, outputLen)
		_, err = io.ReadFull(stream, responses[i].Output)
		if err != nil {
			return nil, limit, errors.AddContext(err, "error reading response output data")
		}

		// If the response contains an error we are done.
		if responses[i].Error != nil {
			return responses, limit, nil
		}
	}

	// The next read should return io.EOF since the host closes the connection
	// after the RPC is done.
	err = modules.RPCRead(stream, struct{}{})
	if !errors.Contains(err, io.ErrClosedPipe) {
		return nil, limit, err
	}
	return responses, limit, nil
}

// FundEphemeralAccount will deposit the given amount in the pair's ephemeral
// account using the pair's file contract to provide payment and the given price
// table.
func (p *renterHostPair) FundEphemeralAccount(amount types.Currency, updatePriceTable bool) (modules.FundAccountResponse, error) {
	var err error

	pt := p.PriceTable()
	if updatePriceTable {
		pt, err = p.FetchPriceTable()
		if err != nil {
			return modules.FundAccountResponse{}, err
		}
	}

	// create stream
	stream := p.newStream()
	defer stream.Close()

	// Write RPC ID.
	err = modules.RPCWrite(stream, modules.RPCFundAccount)
	if err != nil {
		return modules.FundAccountResponse{}, err
	}

	// Write price table id.
	err = modules.RPCWrite(stream, pt.UID)
	if err != nil {
		return modules.FundAccountResponse{}, err
	}

	// send fund account request
	req := modules.FundAccountRequest{Account: p.staticAccountID}
	err = modules.RPCWrite(stream, req)
	if err != nil {
		return modules.FundAccountResponse{}, err
	}

	// Pay by contract.
	err = p.payByContract(stream, amount, modules.ZeroAccountID)
	if err != nil {
		return modules.FundAccountResponse{}, err
	}

	// receive FundAccountResponse
	var resp modules.FundAccountResponse
	err = modules.RPCRead(stream, &resp)
	if err != nil {
		return modules.FundAccountResponse{}, err
	}
	return resp, nil
}

// newStream opens a stream to the pair's host and returns it
func (p *renterHostPair) newStream() siamux.Stream {
	host := p.ht.host
	hes := host.ExternalSettings()

	pk := modules.SiaPKToMuxPK(host.publicKey)
	address := fmt.Sprintf("%s:%s", hes.NetAddress.Host(), hes.SiaMuxPort)
	subscriber := modules.HostSiaMuxSubscriberName

	stream, err := p.staticRenterMux.NewStream(subscriber, address, pk)
	if err != nil {
		panic(err)
	}
	return stream
}

// paymentRevision returns a new revision that transfer the given amount to the
// host. Returns the payment revision together with a signature signed by the
// pair's renter.
func (p *renterHostPair) paymentRevision(amount types.Currency) (types.FileContractRevision, crypto.Signature, error) {
	updated, err := p.ht.host.managedGetStorageObligation(p.staticFCID)
	if err != nil {
		return types.FileContractRevision{}, crypto.Signature{}, err
	}

	recent, err := updated.recentRevision()
	if err != nil {
		return types.FileContractRevision{}, crypto.Signature{}, err
	}

	rev, err := recent.PaymentRevision(amount)
	if err != nil {
		return types.FileContractRevision{}, crypto.Signature{}, err
	}

	return rev, p.sign(rev), nil
}

// payByContract is a helper that creates a payment revision and uses it to pay
// the specified amount. It will also verify the signature of the returned
// response.
func (p *renterHostPair) payByContract(stream siamux.Stream, amount types.Currency, refundAccount modules.AccountID) error {
	// create the revision.
	revision, sig, err := p.paymentRevision(amount)
	if err != nil {
		return err
	}

	// send PaymentRequest & PayByContractRequest
	pRequest := modules.PaymentRequest{Type: modules.PayByContract}
	pbcRequest := newPayByContractRequest(revision, sig, refundAccount)
	err = modules.RPCWriteAll(stream, pRequest, pbcRequest)
	if err != nil {
		return err
	}

	// receive PayByContractResponse
	var payByResponse modules.PayByContractResponse
	err = modules.RPCRead(stream, &payByResponse)
	if err != nil {
		return err
	}

	// verify the host signature
	if err := crypto.VerifyHash(crypto.HashAll(revision), p.ht.host.secretKey.PublicKey(), payByResponse.Signature); err != nil {
		return errors.New("could not verify host signature")
	}
	return nil
}

// payByEphemeralAccount is a helper that makes payment using the pair's EA.
func (p *renterHostPair) payByEphemeralAccount(stream siamux.Stream, amount types.Currency) (modules.PayByEphemeralAccountResponse, error) {
	// Send the payment request.
	err := modules.RPCWrite(stream, modules.PaymentRequest{Type: modules.PayByEphemeralAccount})
	if err != nil {
		return modules.PayByEphemeralAccountResponse{}, err
	}

	// Send the payment details.
	pbear := newPayByEphemeralAccountRequest(p.staticAccountID, p.ht.host.BlockHeight()+6, amount, p.staticAccountKey)
	err = modules.RPCWrite(stream, pbear)
	if err != nil {
		return modules.PayByEphemeralAccountResponse{}, err
	}

	// Receive payment confirmation.
	var resp modules.PayByEphemeralAccountResponse
	err = modules.RPCRead(stream, &resp)
	if err != nil {
		return modules.PayByEphemeralAccountResponse{}, err
	}
	return resp, nil
}

// sign returns the renter's signature of the given revision
func (p *renterHostPair) sign(rev types.FileContractRevision) crypto.Signature {
	signedTxn := types.Transaction{
		FileContractRevisions: []types.FileContractRevision{rev},
		TransactionSignatures: []types.TransactionSignature{{
			ParentID:       crypto.Hash(rev.ParentID),
			CoveredFields:  types.CoveredFields{FileContractRevisions: []uint64{0}},
			PublicKeyIndex: 0,
		}},
	}
	hash := signedTxn.SigHash(0, p.ht.host.BlockHeight())
	return crypto.SignHash(hash, p.staticRenterSK)
}

// UpdatePriceTable runs the UpdatePriceTableRPC on the host and sets the price
// table on the pair
func (p *renterHostPair) UpdatePriceTable(payByFC bool) error {
	stream := p.newStream()
	defer stream.Close()

	// initiate the RPC
	err := modules.RPCWrite(stream, modules.RPCUpdatePriceTable)
	if err != nil {
		return err
	}

	// receive the price table response
	var update modules.RPCUpdatePriceTableResponse
	err = modules.RPCRead(stream, &update)
	if err != nil {
		return err
	}

	var pt modules.RPCPriceTable
	err = json.Unmarshal(update.PriceTableJSON, &pt)
	if err != nil {
		return err
	}

	if payByFC {
		err = p.payByContract(stream, pt.UpdatePriceTableCost, p.staticAccountID)
		if err != nil {
			return err
		}
	} else {
		_, err = p.payByEphemeralAccount(stream, pt.UpdatePriceTableCost)
		if err != nil {
			return err
		}
	}

	// update the price table
	p.SetPriceTable(&pt)

	// expect clean stream close
	err = modules.RPCRead(stream, struct{}{})
	if !errors.Contains(err, io.ErrClosedPipe) {
		return err
	}

	return nil
}

// verify verifies the given signature was made by the host
func (p *renterHostPair) verify(hash crypto.Hash, signature crypto.Signature) error {
	var hpk crypto.PublicKey
	copy(hpk[:], p.ht.host.PublicKey().Key)
	return crypto.VerifyHash(hash, hpk, signature)
}

// TestHostInitialization checks that the host initializes to sensible default
// values.
func TestHostInitialization(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()

	// create a blank host tester
	ht, err := blankHostTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer ht.Close()

	// verify its initial block height is zero
	if ht.host.blockHeight != 0 {
		t.Fatal("host initialized to the wrong block height")
	}

	// verify its RPC price table was properly initialised
	ht.host.staticPriceTables.mu.RLock()
	defer ht.host.staticPriceTables.mu.RUnlock()
	if reflect.DeepEqual(ht.host.staticPriceTables.current, modules.RPCPriceTable{}) {
		t.Fatal("RPC price table wasn't initialized")
	}
	if ht.host.staticPriceTables.current.Expiry == 0 {
		t.Fatal("RPC price table was not properly initialised")
	}
}

// TestHostMultiClose checks that the host returns an error if Close is called
// multiple times on the host.
func TestHostMultiClose(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	ht, err := newHostTester("TestHostMultiClose")
	if err != nil {
		t.Fatal(err)
	}
	defer ht.Close()

	err = ht.host.Close()
	if err != nil {
		t.Fatal(err)
	}
	err = ht.host.Close()
	if err != siasync.ErrStopped {
		t.Fatal(err)
	}
	err = ht.host.Close()
	if err != siasync.ErrStopped {
		t.Fatal(err)
	}
	// Set ht.host to something non-nil - nil was returned because startup was
	// incomplete. If ht.host is nil at the end of the function, the ht.Close()
	// operation will fail.
	ht.host, err = NewCustomHost(modules.ProdDependencies, ht.cs, ht.gateway, ht.tpool, ht.wallet, ht.mux, "localhost:0", filepath.Join(ht.persistDir, modules.HostDir))
	if err != nil {
		t.Fatal(err)
	}
}

// TestNilValues tries initializing the host with nil values.
func TestNilValues(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()
	ht, err := blankHostTester("TestStartupRescan")
	if err != nil {
		t.Fatal(err)
	}
	defer ht.Close()

	hostDir := filepath.Join(ht.persistDir, modules.HostDir)
	_, err = New(nil, ht.gateway, ht.tpool, ht.wallet, ht.mux, "localhost:0", hostDir)
	if err != errNilCS {
		t.Fatal("could not trigger errNilCS")
	}
	_, err = New(ht.cs, nil, ht.tpool, ht.wallet, ht.mux, "localhost:0", hostDir)
	if err != errNilGateway {
		t.Fatal("Could not trigger errNilGateay")
	}
	_, err = New(ht.cs, ht.gateway, nil, ht.wallet, ht.mux, "localhost:0", hostDir)
	if err != errNilTpool {
		t.Fatal("could not trigger errNilTpool")
	}
	_, err = New(ht.cs, ht.gateway, ht.tpool, nil, ht.mux, "localhost:0", hostDir)
	if err != errNilWallet {
		t.Fatal("Could not trigger errNilWallet")
	}
}

// TestRenterHostPair tests the newRenterHostPair constructor
func TestRenterHostPair(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()

	rhp, err := newRenterHostPair(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	err = rhp.Close()
	if err != nil {
		t.Fatal(err)
	}
}

// TestSetAndGetInternalSettings checks that the functions for interacting with
// the host's internal settings object are working as expected.
func TestSetAndGetInternalSettings(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()

	ht, err := newHostTester("TestSetAndGetInternalSettings")
	if err != nil {
		t.Fatal(err)
	}
	defer ht.Close()

	// Check the default settings get returned at first call.
	settings := ht.host.InternalSettings()
	if settings.AcceptingContracts != false {
		t.Error("settings retrieval did not return default value")
	}
	if settings.MaxDuration != modules.DefaultMaxDuration {
		t.Error("settings retrieval did not return default value")
	}
	if settings.MaxDownloadBatchSize != uint64(modules.DefaultMaxDownloadBatchSize) {
		t.Error("settings retrieval did not return default value")
	}
	if settings.MaxReviseBatchSize != uint64(modules.DefaultMaxReviseBatchSize) {
		t.Error("settings retrieval did not return default value")
	}
	if settings.NetAddress != "" {
		t.Error("settings retrieval did not return default value")
	}
	if settings.WindowSize != modules.DefaultWindowSize {
		t.Error("settings retrieval did not return default value")
	}
	if !settings.Collateral.Equals(modules.DefaultCollateral) {
		t.Error("settings retrieval did not return default value")
	}
	if !settings.CollateralBudget.Equals(defaultCollateralBudget) {
		t.Error("settings retrieval did not return default value")
	}
	if !settings.MaxCollateral.Equals(modules.DefaultMaxCollateral) {
		t.Error("settings retrieval did not return default value")
	}
	if !settings.MinContractPrice.Equals(modules.DefaultContractPrice) {
		t.Error("settings retrieval did not return default value")
	}
	if !settings.MinDownloadBandwidthPrice.Equals(modules.DefaultDownloadBandwidthPrice) {
		t.Error("settings retrieval did not return default value")
	}
	if !settings.MinStoragePrice.Equals(modules.DefaultStoragePrice) {
		t.Error("settings retrieval did not return default value")
	}
	if !settings.MinUploadBandwidthPrice.Equals(modules.DefaultUploadBandwidthPrice) {
		t.Error("settings retrieval did not return default value")
	}
	if settings.EphemeralAccountExpiry != (defaultEphemeralAccountExpiry) {
		t.Error("settings retrieval did not return default value")
	}
	if !settings.MaxEphemeralAccountBalance.Equals(defaultMaxEphemeralAccountBalance) {
		t.Error("settings retrieval did not return default value")
	}
	if !settings.MaxEphemeralAccountRisk.Equals(defaultMaxEphemeralAccountRisk) {
		t.Error("settings retrieval did not return default value")
	}

	// Check that calling SetInternalSettings with valid settings updates the settings.
	settings.AcceptingContracts = true
	settings.NetAddress = "foo.com:123"
	err = ht.host.SetInternalSettings(settings)
	if err != nil {
		t.Fatal(err)
	}
	settings = ht.host.InternalSettings()
	if settings.AcceptingContracts != true {
		t.Fatal("SetInternalSettings failed to update settings")
	}
	if settings.NetAddress != "foo.com:123" {
		t.Fatal("SetInternalSettings failed to update settings")
	}

	// Check that calling SetInternalSettings with invalid settings does not update the settings.
	settings.NetAddress = "invalid"
	err = ht.host.SetInternalSettings(settings)
	if err == nil {
		t.Fatal("expected SetInternalSettings to error with invalid settings")
	}
	settings = ht.host.InternalSettings()
	if settings.NetAddress != "foo.com:123" {
		t.Fatal("SetInternalSettings should not modify the settings if the new settings are invalid")
	}

	// Reload the host and verify that the altered settings persisted.
	err = ht.host.Close()
	if err != nil {
		t.Fatal(err)
	}
	rebootHost, err := New(ht.cs, ht.gateway, ht.tpool, ht.wallet, ht.mux, "localhost:0", filepath.Join(ht.persistDir, modules.HostDir))
	if err != nil {
		t.Fatal(err)
	}
	rebootSettings := rebootHost.InternalSettings()
	if rebootSettings.AcceptingContracts != settings.AcceptingContracts {
		t.Error("settings retrieval did not return updated value")
	}
	if rebootSettings.NetAddress != settings.NetAddress {
		t.Error("settings retrieval did not return updated value")
	}

	// Set ht.host to 'rebootHost' so that the 'ht.Close()' method will close
	// everything cleanly.
	ht.host = rebootHost
}

/*
// TestSetAndGetSettings checks that the functions for interacting with the
// hosts settings object are working as expected.
func TestSetAndGetSettings(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	ht, err := newHostTester("TestSetAndGetSettings")
	if err != nil {
		t.Fatal(err)
	}
	defer ht.Close()

	// Check the default settings get returned at first call.
	settings := ht.host.Settings()
	if settings.MaxDuration != modules.DefaultMaxDuration {
		t.Error("settings retrieval did not return default value")
	}
	if settings.WindowSize != modules.DefaultWindowSize {
		t.Error("settings retrieval did not return default value")
	}
	if settings.Price.Cmp(defaultPrice) != 0 {
		t.Error("settings retrieval did not return default value")
	}
	if settings.Collateral.Cmp(modules.DefaultCollateral) != 0 {
		t.Error("settings retrieval did not return default value")
	}

	// Submit updated settings and check that the changes stuck.
	settings.TotalStorage += 15
	settings.MaxDuration += 16
	settings.WindowSize += 17
	settings.Price = settings.Price.Add(types.NewCurrency64(18))
	settings.Collateral = settings.Collateral.Add(types.NewCurrency64(19))
	err = ht.host.SetSettings(settings)
	if err != nil {
		t.Fatal(err)
	}
	newSettings := ht.host.Settings()
	if settings.MaxDuration != newSettings.MaxDuration {
		t.Error("settings retrieval did not return updated value")
	}
	if settings.WindowSize != newSettings.WindowSize {
		t.Error("settings retrieval did not return updated value")
	}
	if settings.Price.Cmp(newSettings.Price) != 0 {
		t.Error("settings retrieval did not return updated value")
	}
	if settings.Collateral.Cmp(newSettings.Collateral) != 0 {
		t.Error("settings retrieval did not return updated value")
	}

	// Reload the host and verify that the altered settings persisted.
	err = ht.host.Close()
	if err != nil {
		t.Fatal(err)
	}
	rebootHost, err := New(ht.cs, ht.tpool, ht.wallet, ht.mux, "localhost:0", filepath.Join(ht.persistDir, modules.HostDir))
	if err != nil {
		t.Fatal(err)
	}
	rebootSettings := rebootHost.Settings()
	if settings.TotalStorage != rebootSettings.TotalStorage {
		t.Error("settings retrieval did not return updated value")
	}
	if settings.MaxDuration != rebootSettings.MaxDuration {
		t.Error("settings retrieval did not return updated value")
	}
	if settings.WindowSize != rebootSettings.WindowSize {
		t.Error("settings retrieval did not return updated value")
	}
	if settings.Price.Cmp(rebootSettings.Price) != 0 {
		t.Error("settings retrieval did not return updated value")
	}
	if settings.Collateral.Cmp(rebootSettings.Collateral) != 0 {
		t.Error("settings retrieval did not return updated value")
	}
}

// TestPersistentSettings checks that settings persist between instances of the
// host.
func TestPersistentSettings(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	ht, err := newHostTester("TestSetPersistentSettings")
	if err != nil {
		t.Fatal(err)
	}
	defer ht.Close()

	// Submit updated settings.
	settings := ht.host.Settings()
	settings.TotalStorage += 25
	settings.MaxDuration += 36
	settings.WindowSize += 47
	settings.Price = settings.Price.Add(types.NewCurrency64(38))
	settings.Collateral = settings.Collateral.Add(types.NewCurrency64(99))
	err = ht.host.SetSettings(settings)
	if err != nil {
		t.Fatal(err)
	}

	// Reboot the host and verify that the new settings stuck.
	err = ht.host.Close() // host saves upon closing
	if err != nil {
		t.Fatal(err)
	}
	h, err := New(ht.cs, ht.tpool, ht.wallet, ht.mux, "localhost:0", filepath.Join(ht.persistDir, modules.HostDir))
	if err != nil {
		t.Fatal(err)
	}
	newSettings := h.Settings()
	if settings.TotalStorage != newSettings.TotalStorage {
		t.Error("settings retrieval did not return updated value:", settings.TotalStorage, "vs", newSettings.TotalStorage)
	}
	if settings.MaxDuration != newSettings.MaxDuration {
		t.Error("settings retrieval did not return updated value")
	}
	if settings.WindowSize != newSettings.WindowSize {
		t.Error("settings retrieval did not return updated value")
	}
	if settings.Price.Cmp(newSettings.Price) != 0 {
		t.Error("settings retrieval did not return updated value")
	}
	if settings.Collateral.Cmp(newSettings.Collateral) != 0 {
		t.Error("settings retrieval did not return updated value")
	}
}
*/
