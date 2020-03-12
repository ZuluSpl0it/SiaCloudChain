package proto

import (
	"gitlab.com/scpcorp/ScPrime/build"
	"gitlab.com/scpcorp/ScPrime/crypto"
	"gitlab.com/scpcorp/ScPrime/modules"
	"gitlab.com/scpcorp/ScPrime/types"
	"gitlab.com/scpcorp/ScPrime/types/typesutil"

	"gitlab.com/NebulousLabs/errors"
)

// Renew negotiates a new contract for data already stored with a host, and
// submits the new contract transaction to tpool. The new contract is added to
// the ContractSet and its metadata is returned.
func (cs *ContractSet) Renew(oldContract *SafeContract, params ContractParams, txnBuilder transactionBuilder, tpool transactionPool, hdb hostDB, cancel <-chan struct{}) (rc modules.RenterContract, formationTxnSet []types.Transaction, sweepTxn types.Transaction, sweepParents []types.Transaction, err error) {
	// Check that the host version is high enough as belt-and-suspenders. This
	// should never happen, because hosts with old versions should be blacklisted
	// by the contractor.
	if build.VersionCmp(params.Host.Version, modules.MinimumSupportedRenterHostProtocolVersion) < 0 {
		return modules.RenterContract{}, nil, types.Transaction{}, nil, ErrBadHostVersion
	}

	// for convenience
	contract := oldContract.header

	// Extract vars from params, for convenience.
	allowance, host, funding, startHeight, endHeight, refundAddress := params.Allowance, params.Host, params.Funding, params.StartHeight, params.EndHeight, params.RefundAddress
	ourSK := contract.SecretKey
	lastRev := contract.LastRevision()

	// Create transaction builder for this renewal with the correct amount of
	// funding.
	err = txnBuilder.FundSiacoins(funding)
	if err != nil {
		return modules.RenterContract{}, nil, types.Transaction{}, nil, err
	}

	// Make a copy of the transaction builder so far, to be used to by the watchdog
	// to double spend these inputs in case the contract never appears on chain.
	sweepBuilder := txnBuilder.Copy()
	if err != nil {
		return modules.RenterContract{}, nil, types.Transaction{}, nil, err
	}
	// Add an output that sends all fund back to the refundAddress.
	// Note that in order to send this transaction, a miner fee will have to be subtracted.
	output := types.SiacoinOutput{
		Value:      funding,
		UnlockHash: refundAddress,
	}
	sweepBuilder.AddSiacoinOutput(output)
	sweepTxn, sweepParents = sweepBuilder.View()

	// Calculate additional basePrice and baseCollateral. If the contract height
	// did not increase, basePrice and baseCollateral are zero.
	var basePrice, baseCollateral types.Currency
	if endHeight+host.WindowSize > lastRev.NewWindowEnd {
		timeExtension := uint64((endHeight + host.WindowSize) - lastRev.NewWindowEnd)
		basePrice = host.StoragePrice.Mul64(lastRev.NewFileSize).Mul64(timeExtension)    // cost of already uploaded data that needs to be covered by the renewed contract.
		baseCollateral = host.Collateral.Mul64(lastRev.NewFileSize).Mul64(timeExtension) // same as basePrice.
	}

	// Calculate the anticipated transaction fee.
	_, maxFee := tpool.FeeEstimation()
	txnFee := maxFee.Mul64(modules.EstimatedFileContractTransactionSetSize)

	// Calculate the payouts for the renter, host, and whole contract.
	period := endHeight - startHeight
	renterPayout, hostPayout, hostCollateral, err := modules.RenterPayoutsPreTax(host, funding, txnFee, basePrice, baseCollateral, period, allowance.ExpectedStorage/allowance.Hosts)
	if err != nil {
		return modules.RenterContract{}, nil, types.Transaction{}, nil, err
	}
	totalPayout := renterPayout.Add(hostPayout)

	// check for negative currency
	if hostCollateral.Cmp(baseCollateral) < 0 {
		baseCollateral = hostCollateral
	}
	if types.PostTax(startHeight, totalPayout).Cmp(hostPayout) < 0 {
		return modules.RenterContract{}, nil, types.Transaction{}, nil, errors.New("insufficient funds to pay both siafund fee and also host payout")
	}

	// create file contract
	fc := types.FileContract{
		FileSize:       lastRev.NewFileSize,
		FileMerkleRoot: lastRev.NewFileMerkleRoot,
		WindowStart:    endHeight,
		WindowEnd:      endHeight + host.WindowSize,
		Payout:         totalPayout,
		UnlockHash:     lastRev.NewUnlockHash,
		RevisionNumber: 0,
		ValidProofOutputs: []types.SiacoinOutput{
			// renter
			{Value: types.PostTax(startHeight, totalPayout).Sub(hostPayout), UnlockHash: refundAddress},
			// host
			{Value: hostPayout, UnlockHash: host.UnlockHash},
		},
		MissedProofOutputs: []types.SiacoinOutput{
			// renter
			{Value: types.PostTax(startHeight, totalPayout).Sub(hostPayout), UnlockHash: refundAddress},
			// host gets its unused collateral back, plus the contract price
			{Value: hostCollateral.Sub(baseCollateral).Add(host.ContractPrice), UnlockHash: host.UnlockHash},
			// void gets the spent storage fees, plus the collateral being risked
			{Value: basePrice.Add(baseCollateral), UnlockHash: types.UnlockHash{}},
		},
	}

	// Add fc.
	txnBuilder.AddFileContract(fc)
	// add miner fee
	txnBuilder.AddMinerFee(txnFee)
	// Add FileContract identifier.
	fcTxn, _ := txnBuilder.View()
	si, hk := PrefixedSignedIdentifier(params.RenterSeed, fcTxn, host.PublicKey)
	_ = txnBuilder.AddArbitraryData(append(si[:], hk[:]...))

	// Create initial transaction set. Before sending the transaction set to the
	// host, ensure that all transactions which may be necessary to get accepted
	// into the transaction pool are included. Also ensure that only the minimum
	// set of transactions is supplied, if there are non-necessary transactions
	// included the chance of a double spend or poor propagation increases.
	txn, parentTxns := txnBuilder.View()
	unconfirmedParents, err := txnBuilder.UnconfirmedParents()
	if err != nil {
		return modules.RenterContract{}, nil, types.Transaction{}, nil, err
	}
	txnSet := append(unconfirmedParents, parentTxns...)
	txnSet = typesutil.MinimumTransactionSet([]types.Transaction{txn}, txnSet)

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
		return modules.RenterContract{}, nil, types.Transaction{}, nil, err
	}
	defer s.Close()
	// Lock the contract and resynchronize if necessary
	rev, sigs, err := s.Lock(contract.ID(), contract.SecretKey)
	if err != nil {
		return modules.RenterContract{}, nil, types.Transaction{}, nil, err
	} else if err := oldContract.managedSyncRevision(rev, sigs); err != nil {
		return modules.RenterContract{}, nil, types.Transaction{}, nil, err
	}

	// Send the RenewContract request.
	req := modules.LoopRenewContractRequest{
		Transactions: txnSet,
		RenterKey:    lastRev.UnlockConditions.PublicKeys[0],
	}
	if err := s.writeRequest(modules.RPCLoopRenewContract, req); err != nil {
		return modules.RenterContract{}, nil, types.Transaction{}, nil, err
	}

	// Read the host's response.
	var resp modules.LoopContractAdditions
	if err := s.readResponse(&resp, modules.RPCMinLen); err != nil {
		return modules.RenterContract{}, nil, types.Transaction{}, nil, err
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
		err = errors.New("failed to sign transaction: " + err.Error())
		modules.WriteRPCResponse(s.conn, s.aead, nil, err)
		return modules.RenterContract{}, nil, types.Transaction{}, nil, err
	}

	// calculate signatures added by the transaction builder
	var addedSignatures []types.TransactionSignature
	_, _, _, addedSignatureIndices := txnBuilder.ViewAdded()
	for _, i := range addedSignatureIndices {
		addedSignatures = append(addedSignatures, signedTxnSet[len(signedTxnSet)-1].TransactionSignatures[i])
	}

	// create initial (no-op) revision, transaction, and signature
	initRevision := types.FileContractRevision{
		ParentID:          signedTxnSet[len(signedTxnSet)-1].FileContractID(0),
		UnlockConditions:  lastRev.UnlockConditions,
		NewRevisionNumber: 1,

		NewFileSize:           fc.FileSize,
		NewFileMerkleRoot:     fc.FileMerkleRoot,
		NewWindowStart:        fc.WindowStart,
		NewWindowEnd:          fc.WindowEnd,
		NewValidProofOutputs:  fc.ValidProofOutputs,
		NewMissedProofOutputs: fc.MissedProofOutputs,
		NewUnlockHash:         fc.UnlockHash,
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
	encodedSig := crypto.SignHash(revisionTxn.SigHash(0, startHeight), ourSK)
	revisionTxn.TransactionSignatures[0].Signature = encodedSig[:]

	// Send acceptance and signatures
	renterSigs := modules.LoopContractSignatures{
		ContractSignatures: addedSignatures,
		RevisionSignature:  revisionTxn.TransactionSignatures[0],
	}
	if err := modules.WriteRPCResponse(s.conn, s.aead, renterSigs, nil); err != nil {
		return modules.RenterContract{}, nil, types.Transaction{}, nil, err
	}

	// Read the host acceptance and signatures.
	var hostSigs modules.LoopContractSignatures
	if err := s.readResponse(&hostSigs, modules.RPCMinLen); err != nil {
		return modules.RenterContract{}, nil, types.Transaction{}, nil, err
	}
	for _, sig := range hostSigs.ContractSignatures {
		txnBuilder.AddTransactionSignature(sig)
	}
	revisionTxn.TransactionSignatures = append(revisionTxn.TransactionSignatures, hostSigs.RevisionSignature)

	// Construct the final transaction.
	txn, parentTxns = txnBuilder.View()
	unconfirmedParents, err = txnBuilder.UnconfirmedParents()
	if err != nil {
		return modules.RenterContract{}, nil, types.Transaction{}, nil, err
	}
	txnSet = append(unconfirmedParents, parentTxns...)
	txnSet = typesutil.MinimumTransactionSet([]types.Transaction{txn}, txnSet)

	// Submit to blockchain.
	err = tpool.AcceptTransactionSet(txnSet)
	if err == modules.ErrDuplicateTransactionSet {
		// As long as it made it into the transaction pool, we're good.
		err = nil
	}
	if err != nil {
		return modules.RenterContract{}, nil, types.Transaction{}, nil, err
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
		return modules.RenterContract{}, nil, types.Transaction{}, nil, err
	}

	// Add contract to set.
	meta, err := cs.managedInsertContract(header, oldRoots)
	if err != nil {
		return modules.RenterContract{}, nil, types.Transaction{}, nil, err
	}
	return meta, txnSet, sweepTxn, sweepParents, nil
}
