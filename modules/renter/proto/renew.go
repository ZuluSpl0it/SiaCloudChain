package proto

import (
	"math"

	"gitlab.com/NebulousLabs/errors"

	"gitlab.com/scpcorp/ScPrime/build"
	"gitlab.com/scpcorp/ScPrime/crypto"
	"gitlab.com/scpcorp/ScPrime/modules"
	"gitlab.com/scpcorp/ScPrime/types"
	"gitlab.com/scpcorp/ScPrime/types/typesutil"
)

// Renew negotiates a new contract for data already stored with a host, and
// submits the new contract transaction to tpool. The new contract is added to
// the ContractSet and its metadata is returned.
func (cs *ContractSet) Renew(oldContract *SafeContract, params ContractParams, txnBuilder transactionBuilder, tpool transactionPool, hdb hostDB, cancel <-chan struct{}) (rc modules.RenterContract, formationTxnSet []types.Transaction, err error) {
	// Check that the host version is high enough as belt-and-suspenders. This
	// should never happen, because hosts with old versions should be blacklisted
	// by the contractor.
	if build.VersionCmp(params.Host.Version, modules.MinimumSupportedRenterHostProtocolVersion) < 0 {
		return modules.RenterContract{}, nil, ErrBadHostVersion
	}
	// Choose the appropriate protocol depending on the host version.
	if build.VersionCmp(params.Host.Version, "1.4.3") >= 0 {
		return cs.managedNewRenewAndClear(oldContract, params, txnBuilder, tpool, hdb, cancel)
	}
	return cs.managedNewRenew(oldContract, params, txnBuilder, tpool, hdb, cancel)
}

func (cs *ContractSet) managedNewRenew(oldContract *SafeContract, params ContractParams, txnBuilder transactionBuilder, tpool transactionPool, hdb hostDB, cancel <-chan struct{}) (rc modules.RenterContract, formationTxnSet []types.Transaction, err error) {
	// for convenience
	contract := oldContract.header

	// Extract vars from params, for convenience.
	host, funding, startHeight, endHeight := params.Host, params.Funding, params.StartHeight, params.EndHeight
	ourSK := contract.SecretKey
	lastRev := contract.LastRevision()

	// Calculate the anticipated transaction fee.
	_, maxFee := tpool.FeeEstimation()
	txnFee := maxFee.Mul64(modules.EstimatedFileContractTransactionSetSize)

	// Calculate the base cost.
	basePrice, baseCollateral := baseCosts(lastRev, host, endHeight)

	// Create file contract and add it together with the fee to the builder.
	fc, err := createRenewedContract(lastRev, params, txnFee, basePrice, baseCollateral, tpool)
	if err != nil {
		return modules.RenterContract{}, nil, err
	}
	txnBuilder.AddFileContract(fc)
	txnBuilder.AddMinerFee(txnFee)

	// Add FileContract identifier.
	fcTxn, _ := txnBuilder.View()
	si, hk := PrefixedSignedIdentifier(params.RenterSeed, fcTxn, host.PublicKey)
	_ = txnBuilder.AddArbitraryData(append(si[:], hk[:]...))

	// Create initial transaction set.
	txnSet, err := prepareTransactionSet(txnBuilder)
	if err != nil {
		return modules.RenterContract{}, nil, err
	}

	// Increase Successful/Failed interactions accordingly
	defer func() {
		// A revision mismatch might not be the host's fault.
		if err != nil && !IsRevisionMismatch(err) {
			hdb.IncrementFailedInteractions(contract.HostPublicKey())
			err = errors.Extend(err, modules.ErrHostFault)
		} else if err == nil {
			hdb.IncrementSuccessfulInteractions(contract.HostPublicKey())
		}
	}()

	// Initiate protocol.
	s, err := cs.NewRawSession(host, startHeight, hdb, cancel)
	if err != nil {
		return modules.RenterContract{}, nil, err
	}
	defer s.Close()

	// Lock the contract and resynchronize if necessary
	rev, sigs, err := s.Lock(contract.ID(), contract.SecretKey)
	if err != nil {
		return modules.RenterContract{}, nil, err
	} else if err := oldContract.managedSyncRevision(rev, sigs); err != nil {
		return modules.RenterContract{}, nil, err
	}

	// Send the RenewContract request.
	req := modules.LoopRenewContractRequest{
		Transactions: txnSet,
		RenterKey:    lastRev.UnlockConditions.PublicKeys[0],
	}
	if err := s.writeRequest(modules.RPCLoopRenewContract, req); err != nil {
		return modules.RenterContract{}, nil, err
	}

	// Read the host's response.
	var resp modules.LoopContractAdditions
	if err := s.readResponse(&resp, modules.RPCMinLen); err != nil {
		return modules.RenterContract{}, nil, err
	}

	// Incorporate host's modifications.
	txnBuilder.AddParents(resp.Parents)
	for _, input := range resp.Inputs {
		txnBuilder.AddSiacoinInput(input)
	}
	for _, output := range resp.Outputs {
		txnBuilder.AddSiacoinOutput(output)
	}

	// sign the txn
	signedTxnSet, err := txnBuilder.Sign(true)
	if err != nil {
		return modules.RenterContract{}, nil, errors.New("failed to sign transaction: " + err.Error())
	}

	// calculate signatures added by the transaction builder
	var addedSignatures []types.TransactionSignature
	_, _, _, addedSignatureIndices := txnBuilder.ViewAdded()
	for _, i := range addedSignatureIndices {
		addedSignatures = append(addedSignatures, signedTxnSet[len(signedTxnSet)-1].TransactionSignatures[i])
	}

	// Prepare the noop init revision.
	revisionTxn := prepareInitRevisionTxn(lastRev, fc, startHeight, ourSK, signedTxnSet[len(signedTxnSet)-1].FileContractID(0))

	// Send acceptance and signatures
	renterSigs := modules.LoopContractSignatures{
		ContractSignatures: addedSignatures,
		RevisionSignature:  revisionTxn.TransactionSignatures[0],
	}
	if err := modules.WriteRPCResponse(s.conn, s.aead, renterSigs, nil); err != nil {
		return modules.RenterContract{}, nil, err
	}

	// Read the host acceptance and signatures.
	var hostSigs modules.LoopContractSignatures
	if err := s.readResponse(&hostSigs, modules.RPCMinLen); err != nil {
		return modules.RenterContract{}, nil, err
	}
	for _, sig := range hostSigs.ContractSignatures {
		txnBuilder.AddTransactionSignature(sig)
	}
	revisionTxn.TransactionSignatures = append(revisionTxn.TransactionSignatures, hostSigs.RevisionSignature)

	// Construct the final transaction.
	txnSet, err = prepareTransactionSet(txnBuilder)
	if err != nil {
		return modules.RenterContract{}, nil, err
	}

	// Submit to blockchain.
	err = tpool.AcceptTransactionSet(txnSet)
	if err == modules.ErrDuplicateTransactionSet {
		// As long as it made it into the transaction pool, we're good.
		err = nil
	}
	if err != nil {
		return modules.RenterContract{}, nil, err
	}

	// Construct contract header.
	header := contractHeader{
		Transaction:     revisionTxn,
		SecretKey:       ourSK,
		StartHeight:     startHeight,
		TotalCost:       funding,
		ContractFee:     host.ContractPrice,
		TxnFee:          txnFee,
		SiafundFee:      types.Tax(startHeight, fc.Payout),
		StorageSpending: basePrice,
		Utility: modules.ContractUtility{
			GoodForUpload: true,
			GoodForRenew:  true,
		},
	}

	// Get old roots
	oldRoots, err := oldContract.merkleRoots.merkleRoots()
	if err != nil {
		return modules.RenterContract{}, nil, err
	}

	// Add contract to set.
	meta, err := cs.managedInsertContract(header, oldRoots)
	if err != nil {
		return modules.RenterContract{}, nil, err
	}
	return meta, txnSet, nil
}

// managedNewRenewAndClear uses the new RPC to renew a contract, creating a new
// contract that is identical to the old one, and then clears the old one to be
// empty.
func (cs *ContractSet) managedNewRenewAndClear(oldContract *SafeContract, params ContractParams, txnBuilder transactionBuilder, tpool transactionPool, hdb hostDB, cancel <-chan struct{}) (rc modules.RenterContract, formationTxnSet []types.Transaction, err error) {
	// for convenience
	contract := oldContract.header

	// Extract vars from params, for convenience.
	host, funding, startHeight, endHeight := params.Host, params.Funding, params.StartHeight, params.EndHeight
	ourSK := contract.SecretKey
	lastRev := contract.LastRevision()

	// Calculate the anticipated transaction fee.
	_, maxFee := tpool.FeeEstimation()
	txnFee := maxFee.Mul64(modules.EstimatedFileContractTransactionSetSize)

	// Calculate the base cost.
	basePrice, baseCollateral := baseCosts(lastRev, host, endHeight)

	// Create file contract and add it together with the fee to the builder.
	fc, err := createRenewedContract(lastRev, params, txnFee, basePrice, baseCollateral, tpool)
	if err != nil {
		return modules.RenterContract{}, nil, err
	}
	txnBuilder.AddFileContract(fc)
	txnBuilder.AddMinerFee(txnFee)

	// Add FileContract identifier.
	fcTxn, _ := txnBuilder.View()
	si, hk := PrefixedSignedIdentifier(params.RenterSeed, fcTxn, host.PublicKey)
	_ = txnBuilder.AddArbitraryData(append(si[:], hk[:]...))

	// Create initial transaction set.
	txnSet, err := prepareTransactionSet(txnBuilder)
	if err != nil {
		return modules.RenterContract{}, nil, err
	}

	// Increase Successful/Failed interactions accordingly
	defer func() {
		// A revision mismatch might not be the host's fault.
		if err != nil && !IsRevisionMismatch(err) {
			hdb.IncrementFailedInteractions(contract.HostPublicKey())
			err = errors.Extend(err, modules.ErrHostFault)
		} else if err == nil {
			hdb.IncrementSuccessfulInteractions(contract.HostPublicKey())
		}
	}()

	// Initiate protocol.
	s, err := cs.NewRawSession(host, startHeight, hdb, cancel)
	if err != nil {
		return modules.RenterContract{}, nil, err
	}
	defer s.Close()

	// Lock the contract and resynchronize if necessary
	rev, sigs, err := s.Lock(contract.ID(), contract.SecretKey)
	if err != nil {
		return modules.RenterContract{}, nil, err
	} else if err := oldContract.managedSyncRevision(rev, sigs); err != nil {
		return modules.RenterContract{}, nil, err
	}

	// Create the final revision of the old contract.
	bandwidthCost := host.BaseRPCPrice
	finalRev, err := prepareFinalRevision(contract, bandwidthCost)
	if err != nil {
		return modules.RenterContract{}, nil, errors.AddContext(err, "Unable to create final revision")
	}

	// Create the RenewContract request.
	req := modules.LoopRenewAndClearContractRequest{
		Transactions: txnSet,
		RenterKey:    lastRev.UnlockConditions.PublicKeys[0],
	}
	for _, vpo := range finalRev.NewValidProofOutputs {
		req.FinalValidProofValues = append(req.FinalValidProofValues, vpo.Value)
	}
	for _, mpo := range finalRev.NewMissedProofOutputs {
		req.FinalMissedProofValues = append(req.FinalMissedProofValues, mpo.Value)
	}

	// Send the request.
	if err := s.writeRequest(modules.RPCLoopRenewClearContract, req); err != nil {
		return modules.RenterContract{}, nil, err
	}

	// Record the changes we are about to make to the contract.
	walTxn, err := oldContract.managedRecordClearContractIntent(finalRev, bandwidthCost)
	if err != nil {
		return modules.RenterContract{}, nil, err
	}

	// Read the host's response.
	var resp modules.LoopContractAdditions
	if err := s.readResponse(&resp, modules.RPCMinLen); err != nil {
		return modules.RenterContract{}, nil, err
	}

	// Incorporate host's modifications.
	txnBuilder.AddParents(resp.Parents)
	for _, input := range resp.Inputs {
		txnBuilder.AddSiacoinInput(input)
	}
	for _, output := range resp.Outputs {
		txnBuilder.AddSiacoinOutput(output)
	}

	// sign the final revision of the old contract.
	rev.NewFileMerkleRoot = crypto.Hash{}
	finalRevTxn := types.Transaction{
		FileContractRevisions: []types.FileContractRevision{finalRev},
		TransactionSignatures: []types.TransactionSignature{
			{
				ParentID:       crypto.Hash(finalRev.ParentID),
				CoveredFields:  types.CoveredFields{FileContractRevisions: []uint64{0}},
				PublicKeyIndex: 0, // renter key is always first -- see formContract
			},
			{
				ParentID:       crypto.Hash(finalRev.ParentID),
				PublicKeyIndex: 1,
				CoveredFields:  types.CoveredFields{FileContractRevisions: []uint64{0}},
				Signature:      nil, // to be provided by host
			},
		},
	}
	finalRevSig := crypto.SignHash(finalRevTxn.SigHash(0, s.height), contract.SecretKey)
	finalRevTxn.TransactionSignatures[0].Signature = finalRevSig[:]

	// sign the txn
	signedTxnSet, err := txnBuilder.Sign(true)
	if err != nil {
		err = errors.New("failed to sign transaction: " + err.Error())
		modules.WriteRPCResponse(s.conn, s.aead, nil, err)
		return modules.RenterContract{}, nil, err
	}

	// calculate signatures added by the transaction builder
	var addedSignatures []types.TransactionSignature
	_, _, _, addedSignatureIndices := txnBuilder.ViewAdded()
	for _, i := range addedSignatureIndices {
		addedSignatures = append(addedSignatures, signedTxnSet[len(signedTxnSet)-1].TransactionSignatures[i])
	}

	// create initial (no-op) revision, transaction, and signature
	revisionTxn := prepareInitRevisionTxn(lastRev, fc, startHeight, ourSK, signedTxnSet[len(signedTxnSet)-1].FileContractID(0))

	// Send acceptance and signatures
	renterSigs := modules.LoopRenewAndClearContractSignatures{
		ContractSignatures: addedSignatures,
		RevisionSignature:  revisionTxn.TransactionSignatures[0],

		FinalRevisionSignature: finalRevSig[:],
	}
	if err := modules.WriteRPCResponse(s.conn, s.aead, renterSigs, nil); err != nil {
		return modules.RenterContract{}, nil, err
	}

	// Read the host acceptance and signatures.
	var hostSigs modules.LoopRenewAndClearContractSignatures
	if err := s.readResponse(&hostSigs, modules.RPCMinLen); err != nil {
		return modules.RenterContract{}, nil, err
	}
	for _, sig := range hostSigs.ContractSignatures {
		txnBuilder.AddTransactionSignature(sig)
	}
	revisionTxn.TransactionSignatures = append(revisionTxn.TransactionSignatures, hostSigs.RevisionSignature)
	finalRevTxn.TransactionSignatures[1].Signature = hostSigs.FinalRevisionSignature

	// Construct the final transaction.
	txnSet, err = prepareTransactionSet(txnBuilder)
	if err != nil {
		return modules.RenterContract{}, nil, err
	}

	// Submit to blockchain.
	err = tpool.AcceptTransactionSet(txnSet)
	if err == modules.ErrDuplicateTransactionSet {
		// As long as it made it into the transaction pool, we're good.
		err = nil
	}
	if err != nil {
		return modules.RenterContract{}, nil, err
	}
	err = tpool.AcceptTransactionSet([]types.Transaction{finalRevTxn})
	if err == modules.ErrDuplicateTransactionSet {
		// As long as it made it into the transaction pool, we're good.
		err = nil
	}
	if err != nil {
		return modules.RenterContract{}, nil, err
	}

	// Construct contract header.
	header := contractHeader{
		Transaction:     revisionTxn,
		SecretKey:       ourSK,
		StartHeight:     startHeight,
		TotalCost:       funding,
		ContractFee:     host.ContractPrice,
		TxnFee:          txnFee,
		SiafundFee:      types.Tax(startHeight, fc.Payout),
		StorageSpending: basePrice,
		Utility: modules.ContractUtility{
			GoodForUpload: true,
			GoodForRenew:  true,
		},
	}

	// Get old roots
	oldRoots, err := oldContract.merkleRoots.merkleRoots()
	if err != nil {
		return modules.RenterContract{}, nil, err
	}

	// Add contract to set.
	meta, err := cs.managedInsertContract(header, oldRoots)
	if err != nil {
		return modules.RenterContract{}, nil, err
	}
	// Commit changes to old contract.
	if err := oldContract.managedCommitClearContract(walTxn, finalRevTxn, bandwidthCost); err != nil {
		return modules.RenterContract{}, nil, err
	}
	return meta, txnSet, nil
}

// baseCosts computes the base costs for renewing a contract.
func baseCosts(lastRev types.FileContractRevision, host modules.HostDBEntry, endHeight types.BlockHeight) (basePrice, baseCollateral types.Currency) {
	// If the contract height did not increase, basePrice and baseCollateral are
	// zero.
	if endHeight+host.WindowSize > lastRev.NewWindowEnd {
		timeExtension := uint64((endHeight + host.WindowSize) - lastRev.NewWindowEnd)
		basePrice = host.StoragePrice.Mul64(lastRev.NewFileSize).Mul64(timeExtension)    // cost of already uploaded data that needs to be covered by the renewed contract.
		baseCollateral = host.Collateral.Mul64(lastRev.NewFileSize).Mul64(timeExtension) // same as basePrice.
	}
	return
}

// prepareInitRevisionTxn creates the initRevision, places it in a transaction and
// adds the signature or the revision to the transaction.
func prepareInitRevisionTxn(lastRev types.FileContractRevision, newContract types.FileContract, startHeight types.BlockHeight, renterSK crypto.SecretKey, parentID types.FileContractID) types.Transaction {
	initRevision := types.FileContractRevision{
		ParentID:          parentID,
		UnlockConditions:  lastRev.UnlockConditions,
		NewRevisionNumber: 1,

		NewFileSize:           newContract.FileSize,
		NewFileMerkleRoot:     newContract.FileMerkleRoot,
		NewWindowStart:        newContract.WindowStart,
		NewWindowEnd:          newContract.WindowEnd,
		NewValidProofOutputs:  newContract.ValidProofOutputs,
		NewMissedProofOutputs: newContract.MissedProofOutputs,
		NewUnlockHash:         newContract.UnlockHash,
	}

	renterRevisionSig := types.TransactionSignature{
		ParentID:       crypto.Hash(initRevision.ParentID),
		PublicKeyIndex: 0,
		CoveredFields: types.CoveredFields{
			FileContractRevisions: []uint64{0},
		},
	}

	revisionTxn := types.Transaction{
		FileContractRevisions: []types.FileContractRevision{initRevision},
		TransactionSignatures: []types.TransactionSignature{renterRevisionSig},
	}

	encodedSig := crypto.SignHash(revisionTxn.SigHash(0, startHeight), renterSK)
	revisionTxn.TransactionSignatures[0].Signature = encodedSig[:]
	return revisionTxn
}

// prepareFinalRevision creates a new revision for a contract which transfers
// the given amount of payment, clears the contract and sets the missed outputs
// to equal the valid outputs.
func prepareFinalRevision(contract contractHeader, payment types.Currency) (types.FileContractRevision, error) {
	finalRev, err := contract.LastRevision().PaymentRevision(payment)
	if err != nil {
		return types.FileContractRevision{}, err
	}

	// Clear the revision.
	finalRev.NewFileSize = 0
	finalRev.NewFileMerkleRoot = crypto.Hash{}
	finalRev.NewRevisionNumber = math.MaxUint64

	// The missed proof outputs become the valid ones since the host won't need
	// to provide a storage proof. We need to preserve the void output though.
	finalRev.NewMissedProofOutputs = finalRev.NewValidProofOutputs
	return finalRev, nil
}

// PrepareTransactionSet prepares a transaction set from the given builder to be
// sent to the host. It includes all unconfirmed parents and has all
// non-essential txns trimmed from it.
func prepareTransactionSet(txnBuilder transactionBuilder) ([]types.Transaction, error) {
	txn, parentTxns := txnBuilder.View()
	unconfirmedParents, err := txnBuilder.UnconfirmedParents()
	if err != nil {
		return nil, err
	}
	txnSet := append(unconfirmedParents, parentTxns...)
	txnSet = typesutil.MinimumTransactionSet([]types.Transaction{txn}, txnSet)
	return txnSet, nil
}

// createRenewedContract creates a new contract from another contract's last
// revision given some additional renewal parameters.
func createRenewedContract(lastRev types.FileContractRevision, params ContractParams, txnFee, basePrice, baseCollateral types.Currency, tpool transactionPool) (types.FileContract, error) {
	allowance, startHeight, endHeight, host, funding := params.Allowance, params.StartHeight, params.EndHeight, params.Host, params.Funding

	// Calculate the payouts for the renter, host, and whole contract.
	period := endHeight - startHeight
	renterPayout, hostPayout, hostCollateral, err := modules.RenterPayoutsPreTax(host, funding, txnFee, basePrice, baseCollateral, period, allowance.ExpectedStorage/allowance.Hosts)
	if err != nil {
		return types.FileContract{}, err
	}
	totalPayout := renterPayout.Add(hostPayout)

	// check for negative currency
	if hostCollateral.Cmp(baseCollateral) < 0 {
		baseCollateral = hostCollateral
	}
	if types.PostTax(params.StartHeight, totalPayout).Cmp(hostPayout) < 0 {
		return types.FileContract{}, errors.New("insufficient funds to pay both siafund fee and also host payout")
	}

	return types.FileContract{
		FileSize:       lastRev.NewFileSize,
		FileMerkleRoot: lastRev.NewFileMerkleRoot,
		WindowStart:    params.EndHeight,
		WindowEnd:      params.EndHeight + params.Host.WindowSize,
		Payout:         totalPayout,
		UnlockHash:     lastRev.NewUnlockHash,
		RevisionNumber: 0,
		ValidProofOutputs: []types.SiacoinOutput{
			// renter
			{Value: types.PostTax(params.StartHeight, totalPayout).Sub(hostPayout), UnlockHash: params.RefundAddress},
			// host
			{Value: hostPayout, UnlockHash: params.Host.UnlockHash},
		},
		MissedProofOutputs: []types.SiacoinOutput{
			// renter
			{Value: types.PostTax(params.StartHeight, totalPayout).Sub(hostPayout), UnlockHash: params.RefundAddress},
			// host gets its unused collateral back, plus the contract price
			{Value: hostCollateral.Sub(baseCollateral).Add(params.Host.ContractPrice), UnlockHash: params.Host.UnlockHash},
			// void gets the spent storage fees, plus the collateral being risked
			{Value: basePrice.Add(baseCollateral), UnlockHash: types.UnlockHash{}},
		},
	}, nil
}
