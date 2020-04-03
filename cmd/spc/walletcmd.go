package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"

	"gitlab.com/scpcorp/ScPrime/cmd"
	"gitlab.com/scpcorp/ScPrime/crypto"
	"gitlab.com/scpcorp/ScPrime/encoding"
	"gitlab.com/scpcorp/ScPrime/modules"
	"gitlab.com/scpcorp/ScPrime/modules/wallet"
	"gitlab.com/scpcorp/ScPrime/node/api"
	"gitlab.com/scpcorp/ScPrime/types"

	"github.com/spf13/cobra"
	mnemonics "gitlab.com/NebulousLabs/entropy-mnemonics"
	"gitlab.com/NebulousLabs/errors"
	"golang.org/x/crypto/ssh/terminal"
)

var (
	walletAddressCmd = &cobra.Command{
		Use:   "address",
		Short: "Get a new wallet address",
		Long:  "Generate a new wallet address from the wallet's primary seed.",
		Run:   wrap(walletaddresscmd),
	}

	walletAddressesCmd = &cobra.Command{
		Use:   "addresses",
		Short: "List all addresses",
		Long:  "List all addresses that have been generated by the wallet.",
		Run:   wrap(walletaddressescmd),
	}

	walletBalanceCmd = &cobra.Command{
		Use:   "balance",
		Short: "View wallet balance",
		Long:  "View wallet balance, including confirmed and unconfirmed scprimecoins and scprimefunds.",
		Run:   wrap(walletbalancecmd),
	}

	walletBroadcastCmd = &cobra.Command{
		Use:   "broadcast [txn]",
		Short: "Broadcast a transaction",
		Long: `Broadcast a JSON-encoded transaction to connected peers. The transaction must
be valid. txn may be either JSON, base64, or a file containing either.`,
		Run: wrap(walletbroadcastcmd),
	}

	walletChangepasswordCmd = &cobra.Command{
		Use:   "change-password",
		Short: "Change the wallet password",
		Long:  "Change the encryption password of the wallet, re-encrypting all keys + seeds kept by the wallet.",
		Run:   wrap(walletchangepasswordcmd),
	}

	walletCmd = &cobra.Command{
		Use:   "wallet",
		Short: "Perform wallet actions",
		Long: `Generate a new address, send coins to another wallet, or view info about the wallet.

Units:
The smallest unit of scprimecoins is the hasting. One scprimecoin is 10^27 hastings. Other supported units are:
  pS (pico,  10^-12 SCP)
  nS (nano,  10^-9 SCP)
  uS (micro, 10^-6 SCP)
  mS (milli, 10^-3 SCP)
  SCP
  KS (kilo, 10^3 SCP)
  MS (mega, 10^6 SCP)
  GS (giga, 10^9 SCP)
  TS (tera, 10^12 SCP)`,
		Run: wrap(walletbalancecmd),
	}

	walletInitCmd = &cobra.Command{
		Use:   "init",
		Short: "Initialize and encrypt a new wallet",
		Long: `Generate a new wallet from a randomly generated seed, and encrypt it.
By default the wallet encryption / unlock password is the same as the generated seed.`,
		Run: wrap(walletinitcmd),
	}

	walletInitSeedCmd = &cobra.Command{
		Use:   "init-seed",
		Short: "Initialize and encrypt a new wallet using a pre-existing seed",
		Long:  `Initialize and encrypt a new wallet using a pre-existing seed.`,
		Run:   wrap(walletinitseedcmd),
	}

	walletLoadCmd = &cobra.Command{
		Use:   "load",
		Short: "Load a wallet seed or saipg keyset",
		// Run field is not set, as the load command itself is not a valid command.
		// A subcommand must be provided.
	}

	walletLoadSeedCmd = &cobra.Command{
		Use:   `seed`,
		Short: "Add a seed to the wallet",
		Long:  "Loads an auxiliary seed into the wallet.",
		Run:   wrap(walletloadseedcmd),
	}

	walletLoadSiagCmd = &cobra.Command{
		Use:     `saipg [filepath,...]`,
		Short:   "Load saipg key(s) into the wallet",
		Long:    "Load saipg key(s) into the wallet - typically used for scprimefunds.",
		Example: "spc wallet load saipg key1.siapkey,key2.siapkey",
		Run:     wrap(walletloadsiagcmd),
	}

	walletLockCmd = &cobra.Command{
		Use:   "lock",
		Short: "Lock the wallet",
		Long:  "Lock the wallet, preventing further use",
		Run:   wrap(walletlockcmd),
	}

	walletSeedsCmd = &cobra.Command{
		Use:   "seeds",
		Short: "View information about your seeds",
		Long:  "View your primary and auxiliary wallet seeds.",
		Run:   wrap(walletseedscmd),
	}

	walletSendCmd = &cobra.Command{
		Use:   "send",
		Short: "Send either scprimecoins or scprimefunds to an address",
		Long:  "Send either scprimecoins or scprimefunds to an address",
		// Run field is not set, as the send command itself is not a valid command.
		// A subcommand must be provided.
	}

	walletSendSiacoinsCmd = &cobra.Command{
		Use:   "scprimecoins [amount] [dest]",
		Short: "Send scprimecoins to an address",
		Long: `Send scprimecoins to an address. 'dest' must be a 76-byte hexadecimal address.
'amount' can be specified in units, e.g. 1.23KS. Run 'wallet --help' for a list of units.
If no unit is supplied, hastings will be assumed.

A dynamic transaction fee is applied depending on the size of the transaction and how busy the network is.`,
		Run: wrap(walletsendsiacoinscmd),
	}

	walletSendSiafundsCmd = &cobra.Command{
		Use:   "scprimefunds [amount] [dest]",
		Short: "Send scprimefunds",
		Long: `Send scprimefunds to an address, and transfer the claim scprimecoins to your wallet.
Run 'wallet send --help' to see a list of available units.`,
		Run: wrap(walletsendsiafundscmd),
	}

	walletSignCmd = &cobra.Command{
		Use:   "sign [txn] [tosign]",
		Short: "Sign a transaction",
		Long: `Sign a transaction. If spd is running with an unlocked wallet, the
/wallet/sign API call will be used. Otherwise, sign will prompt for the wallet
seed, and the signing key(s) will be regenerated.

txn may be either JSON, base64, or a file containing either.

tosign is an optional list of indices. Each index corresponds to a
TransactionSignature in the txn that will be filled in. If no indices are
provided, the wallet will fill in every TransactionSignature it has keys for.`,
		Run: walletsigncmd,
	}

	walletSweepCmd = &cobra.Command{
		Use:   "sweep",
		Short: "Sweep scprimecoins and scprimefunds from a seed.",
		Long: `Sweep scprimecoins and scprimefunds from a seed. The outputs belonging to the seed
will be sent to your wallet.`,
		Run: wrap(walletsweepcmd),
	}

	walletTransactionsCmd = &cobra.Command{
		Use:   "transactions",
		Short: "View transactions",
		Long:  "View transactions related to addresses spendable by the wallet, providing a net flow of scprimecoins and scprimefunds for each transaction",
		Run:   wrap(wallettransactionscmd),
	}

	walletUnlockCmd = &cobra.Command{
		Use:   `unlock`,
		Short: "Unlock the wallet",
		Long: `Decrypt and load the wallet into memory.
Automatic unlocking is also supported via environment variable: if the
SCPRIME_WALLET_PASSWORD environment variable is set, the unlock command will
use it instead of displaying the typical interactive prompt.`,
		Run: wrap(walletunlockcmd),
	}
)

/* #nosec */
const askPasswordText = "We need to encrypt the new data using the current wallet password, please provide: "

const currentPasswordText = "Current Password: "
const newPasswordText = "New Password: "
const confirmPasswordText = "Confirm: "

// For an unconfirmed Transaction, the TransactionTimestamp field is set to the
// maximum value of a uint64.
const unconfirmedTransactionTimestamp = ^uint64(0)

// passwordPrompt securely reads a password from stdin.
func passwordPrompt(prompt string) (string, error) {
	fmt.Print(prompt)
	pw, err := terminal.ReadPassword(int(syscall.Stdin))
	fmt.Println()
	return string(pw), err
}

// confirmPassword requests confirmation of a previously-entered password.
func confirmPassword(prev string) error {
	pw, err := passwordPrompt(confirmPasswordText)
	if err != nil {
		return err
	} else if pw != prev {
		return errors.New("passwords do not match")
	}
	return nil
}

// walletaddresscmd fetches a new address from the wallet that will be able to
// receive coins.
func walletaddresscmd() {
	addr, err := httpClient.WalletAddressGet()
	if err != nil {
		die("Could not generate new address:", err)
	}
	fmt.Printf("Created new address: %s\n", addr.Address)
}

// walletaddressescmd fetches the list of addresses that the wallet knows.
func walletaddressescmd() {
	addrs, err := httpClient.WalletAddressesGet()
	if err != nil {
		die("Failed to fetch addresses:", err)
	}
	for _, addr := range addrs.Addresses {
		fmt.Println(addr)
	}
}

// walletchangepasswordcmd changes the password of the wallet.
func walletchangepasswordcmd() {
	currentPassword, err := passwordPrompt(currentPasswordText)
	if err != nil {
		die("Reading password failed:", err)
	}
	newPassword, err := passwordPrompt(newPasswordText)
	if err != nil {
		die("Reading password failed:", err)
	} else if err = confirmPassword(newPassword); err != nil {
		die(err)
	}
	err = httpClient.WalletChangePasswordPost(currentPassword, newPassword)
	if err != nil {
		die("Changing the password failed:", err)
	}
	fmt.Println("Password changed successfully.")
}

// walletinitcmd encrypts the wallet with the given password
func walletinitcmd() {
	var password string
	var err error
	if initPassword {
		password, err = passwordPrompt("Wallet password: ")
		if err != nil {
			die("Reading password failed:", err)
		} else if err = confirmPassword(password); err != nil {
			die(err)
		}
	}
	er, err := httpClient.WalletInitPost(password, initForce)
	if err != nil {
		die("Error when encrypting wallet:", err)
	}
	fmt.Printf("Recovery seed:\n%s\n\n", er.PrimarySeed)
	if initPassword {
		fmt.Printf("Wallet encrypted with given password\n")
	} else {
		fmt.Printf("Wallet encrypted with password:\n%s\n", er.PrimarySeed)
	}
}

// walletinitseedcmd initializes the wallet from a preexisting seed.
func walletinitseedcmd() {
	seed, err := passwordPrompt("Seed: ")
	if err != nil {
		die("Reading seed failed:", err)
	}
	var password string
	if initPassword {
		password, err = passwordPrompt("Wallet password: ")
		if err != nil {
			die("Reading password failed:", err)
		} else if err = confirmPassword(password); err != nil {
			die(err)
		}
	}
	err = httpClient.WalletInitSeedPost(seed, password, initForce)
	if err != nil {
		die("Could not initialize wallet from seed:", err)
	}
	if initPassword {
		fmt.Println("Wallet initialized and encrypted with given password.")
	} else {
		fmt.Println("Wallet initialized and encrypted with seed.")
	}
}

// walletloadseedcmd adds a seed to the wallet's list of seeds
func walletloadseedcmd() {
	seed, err := passwordPrompt("New seed: ")
	if err != nil {
		die("Reading seed failed:", err)
	}
	password, err := passwordPrompt(askPasswordText)
	if err != nil {
		die("Reading password failed:", err)
	}
	err = httpClient.WalletSeedPost(seed, password)
	if err != nil {
		die("Could not add seed:", err)
	}
	fmt.Println("Added Key")
}

// walletloadsiagcmd loads a siag key set into the wallet.
func walletloadsiagcmd(keyfiles string) {
	password, err := passwordPrompt(askPasswordText)
	if err != nil {
		die("Reading password failed:", err)
	}
	err = httpClient.WalletSiagKeyPost(keyfiles, password)
	if err != nil {
		die("Loading saipg key failed:", err)
	}
	fmt.Println("Wallet loading successful.")
}

// walletlockcmd locks the wallet
func walletlockcmd() {
	err := httpClient.WalletLockPost()
	if err != nil {
		die("Could not lock wallet:", err)
	}
}

// walletseedcmd returns the current seed {
func walletseedscmd() {
	seedInfo, err := httpClient.WalletSeedsGet()
	if err != nil {
		die("Error retrieving the current seed:", err)
	}
	fmt.Println("Primary Seed:")
	fmt.Println(seedInfo.PrimarySeed)
	if len(seedInfo.AllSeeds) == 1 {
		// AllSeeds includes the primary seed
		return
	}
	fmt.Println()
	fmt.Println("Auxiliary Seeds:")
	for _, seed := range seedInfo.AllSeeds {
		if seed == seedInfo.PrimarySeed {
			continue
		}
		fmt.Println() // extra newline for readability
		fmt.Println(seed)
	}
}

// walletsendsiacoinscmd sends siacoins to a destination address.
func walletsendsiacoinscmd(amount, dest string) {
	hastings, err := parseCurrency(amount)
	if err != nil {
		die("Could not parse amount:", err)
	}
	var value types.Currency
	if _, err := fmt.Sscan(hastings, &value); err != nil {
		die("Failed to parse amount", err)
	}
	var hash types.UnlockHash
	if _, err := fmt.Sscan(dest, &hash); err != nil {
		die("Failed to parse destination address", err)
	}
	_, err = httpClient.WalletSiacoinsPost(value, hash, walletTxnFeeIncluded)
	if err != nil {
		die("Could not send scprimecoins:", err)
	}
	fmt.Printf("Sent %s hastings to %s\n", hastings, dest)
}

// walletsendsiafundscmd sends siafunds to a destination address.
func walletsendsiafundscmd(amount, dest string) {
	var value types.Currency
	if _, err := fmt.Sscan(amount, &value); err != nil {
		die("Failed to parse amount", err)
	}
	var hash types.UnlockHash
	if _, err := fmt.Sscan(dest, &hash); err != nil {
		die("Failed to parse destination address", err)
	}
	_, err := httpClient.WalletSiafundsPost(value, hash)
	if err != nil {
		die("Could not send scprimefunds:", err)
	}
	fmt.Printf("Sent %s scprimefunds to %s\n", amount, dest)
}

// walletbalancecmd retrieves and displays information about the wallet.
func walletbalancecmd() {
	status, err := httpClient.WalletGet()
	if errors.Contains(err, api.ErrAPICallNotRecognized) {
		// Assume module is not loaded if status command is not recognized.
		fmt.Printf("Wallet:\n  Status: %s\n\n", moduleNotReadyStatus)
		return
	} else if err != nil {
		die("Could not get wallet status:", err)
	}

	fees, err := httpClient.TransactionPoolFeeGet()
	if err != nil {
		die("Could not get fee estimation:", err)
	}
	encStatus := "Unencrypted"
	if status.Encrypted {
		encStatus = "Encrypted"
	}
	if !status.Unlocked {
		fmt.Printf(`Wallet status:
%v, Locked
Unlock the wallet to view balance
`, encStatus)
		return
	}

	unconfirmedBalance := status.ConfirmedSiacoinBalance.Add(status.UnconfirmedIncomingSiacoins).Sub(status.UnconfirmedOutgoingSiacoins)
	var delta string
	if unconfirmedBalance.Cmp(status.ConfirmedSiacoinBalance) >= 0 {
		delta = "+" + currencyUnits(unconfirmedBalance.Sub(status.ConfirmedSiacoinBalance))
	} else {
		delta = "-" + currencyUnits(status.ConfirmedSiacoinBalance.Sub(unconfirmedBalance))
	}

	fmt.Printf(`Wallet status:
%s, Unlocked
Height:              %v
Confirmed Balance:   %v
Unconfirmed Delta:   %v
Exact:               %v H
Scprimefunds:        %v SPF
Scprimefund Claims:  %v H

Estimated Fee:       %v / KB
`, encStatus, status.Height, currencyUnits(status.ConfirmedSiacoinBalance), delta,
		status.ConfirmedSiacoinBalance, status.SiafundBalance, status.SiacoinClaimBalance,
		fees.Maximum.Mul64(1e3).HumanString())
}

// walletbroadcastcmd broadcasts a transaction.
func walletbroadcastcmd(txnStr string) {
	txn, err := parseTxn(txnStr)
	if err != nil {
		die("Could not decode transaction:", err)
	}
	err = httpClient.TransactionPoolRawPost(txn, nil)
	if err != nil {
		die("Could not broadcast transaction:", err)
	}
	fmt.Println("Transaction has been broadcast successfully")
}

// walletsweepcmd sweeps coins and funds from a seed.
func walletsweepcmd() {
	seed, err := passwordPrompt("Seed: ")
	if err != nil {
		die("Reading seed failed:", err)
	}

	swept, err := httpClient.WalletSweepPost(seed)
	if err != nil {
		die("Could not sweep seed:", err)
	}
	fmt.Printf("Swept %v and %v SPF from seed.\n", currencyUnits(swept.Coins), swept.Funds)
}

// walletsigncmd signs a transaction.
func walletsigncmd(cmd *cobra.Command, args []string) {
	if len(args) < 1 {
		cmd.UsageFunc()(cmd)
		os.Exit(exitCodeUsage)
	}

	txn, err := parseTxn(args[0])
	if err != nil {
		die("Could not decode transaction:", err)
	}

	var toSign []crypto.Hash
	for _, arg := range args[1:] {
		index, err := strconv.ParseUint(arg, 10, 32)
		if err != nil {
			die("Invalid signature index", index, "(must be an non-negative integer)")
		} else if index >= uint64(len(txn.TransactionSignatures)) {
			die("Invalid signature index", index, "(transaction only has", len(txn.TransactionSignatures), "signatures)")
		}
		toSign = append(toSign, txn.TransactionSignatures[index].ParentID)
	}

	// try API first
	wspr, err := httpClient.WalletSignPost(txn, toSign)
	if err == nil {
		txn = wspr.Transaction
	} else {
		// if siad is running, but the wallet is locked, assume the user
		// wanted to sign with siad
		if strings.Contains(err.Error(), modules.ErrLockedWallet.Error()) {
			die("Signing via API failed: spd is running, but the wallet is locked.")
		}

		// siad is not running; fallback to offline keygen
		walletsigncmdoffline(&txn, toSign)
	}

	if walletRawTxn {
		base64.NewEncoder(base64.StdEncoding, os.Stdout).Write(encoding.Marshal(txn))
	} else {
		json.NewEncoder(os.Stdout).Encode(txn)
	}
	fmt.Println()
}

// walletsigncmdoffline is a helper for walletsigncmd that handles signing
// transactions without siad.
func walletsigncmdoffline(txn *types.Transaction, toSign []crypto.Hash) {
	fmt.Println("Enter your wallet seed to generate the signing key(s) now and sign without spd.")
	seedString, err := passwordPrompt("Seed: ")
	if err != nil {
		die("Reading seed failed:", err)
	}
	seed, err := modules.StringToSeed(seedString, mnemonics.English)
	if err != nil {
		die("Invalid seed:", err)
	}
	// signing via seed may take a while, since we need to regenerate
	// keys. If it takes longer than a second, print a message to assure
	// the user that this is normal.
	done := make(chan struct{})
	go func() {
		select {
		case <-time.After(time.Second):
			fmt.Println("Generating keys; this may take a few seconds...")
		case <-done:
		}
	}()
	err = wallet.SignTransaction(txn, seed, toSign, 180e3)
	if err != nil {
		die("Failed to sign transaction:", err)
	}
	close(done)
}

// wallettransactionscmd lists all of the transactions related to the wallet,
// providing a net flow of siacoins and siafunds for each.
func wallettransactionscmd() {
	wtg, err := httpClient.WalletTransactionsGet(0, math.MaxInt64)
	if err != nil {
		die("Could not fetch transaction history:", err)
	}
	cg, err := httpClient.ConsensusGet()
	if err != nil {
		die("Could not fetch consensus information:", err)
	}
	fmt.Println("             [timestamp]    [height]                                                   [transaction id]    [net SCP]        [net SPF]")
	txns := append(wtg.ConfirmedTransactions, wtg.UnconfirmedTransactions...)
	sts, err := wallet.ComputeValuedTransactions(txns, cg.Height)
	if err != nil {
		die("Could not compute valued transaction: ", err)
	}
	for _, txn := range sts {
		// Determine the number of outgoing siacoins and siafunds.
		var outgoingSiafunds types.Currency
		for _, input := range txn.Inputs {
			if input.FundType == types.SpecifierSiafundInput && input.WalletAddress {
				outgoingSiafunds = outgoingSiafunds.Add(input.Value)
			}
		}

		// Determine the number of incoming siacoins and siafunds.
		var incomingSiafunds types.Currency
		for _, output := range txn.Outputs {
			if output.FundType == types.SpecifierSiafundOutput && output.WalletAddress {
				incomingSiafunds = incomingSiafunds.Add(output.Value)
			}
		}

		// Convert the siacoins to a float.
		incomingSiacoinsFloat, _ := new(big.Rat).SetFrac(txn.ConfirmedIncomingValue.Big(), types.ScPrimecoinPrecision.Big()).Float64()
		outgoingSiacoinsFloat, _ := new(big.Rat).SetFrac(txn.ConfirmedOutgoingValue.Big(), types.ScPrimecoinPrecision.Big()).Float64()

		// Print the results.
		if uint64(txn.ConfirmationTimestamp) != unconfirmedTransactionTimestamp {
			fmt.Println(time.Unix(int64(txn.ConfirmationTimestamp), 0).Format("2006-01-02 15:04:05-0700"))
		} else {
			fmt.Printf("             unconfirmed")
		}
		if txn.ConfirmationHeight < 1e9 {
			fmt.Printf("%12v", txn.ConfirmationHeight)
		} else {
			fmt.Printf(" unconfirmed")
		}
		fmt.Printf("%67v%15.2f SCP", txn.TransactionID, incomingSiacoinsFloat-outgoingSiacoinsFloat)
		// For siafunds, need to avoid having a negative types.Currency.
		if incomingSiafunds.Cmp(outgoingSiafunds) >= 0 {
			fmt.Printf("%14v SPF\n", incomingSiafunds.Sub(outgoingSiafunds))
		} else {
			fmt.Printf("-%14v SPF\n", outgoingSiafunds.Sub(incomingSiafunds))
		}
	}
}

// walletunlockcmd unlocks a saved wallet
func walletunlockcmd() {
	// try reading from environment variable first, then fallback to
	// interactive method. Also allow overriding auto-unlock via -p
	password := os.Getenv(cmd.SiaWalletPassword)
	if password != "" && !initPassword {
		fmt.Printf("Using %v environment variable", cmd.SiaWalletPassword)
		err := httpClient.WalletUnlockPost(password)
		if err != nil {
			fmt.Println("Automatic unlock failed!")
		} else {
			fmt.Println("Wallet unlocked")
			return
		}
	}
	password, err := passwordPrompt("Wallet password: ")
	if err != nil {
		die("Reading password failed:", err)
	}
	err = httpClient.WalletUnlockPost(password)
	if err != nil {
		die("Could not unlock wallet:", err)
	}
}
