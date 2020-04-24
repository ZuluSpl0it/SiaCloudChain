package host

import (
	"encoding/json"
	"fmt"
	"time"

	"gitlab.com/NebulousLabs/errors"
	"gitlab.com/NebulousLabs/fastrand"
	"gitlab.com/scpcorp/siamux"

	"gitlab.com/scpcorp/ScPrime/modules"
)

// staticRPCUpdatePriceTable returns a copy of the host's current rpc price
// table. These prices are valid for the duration of the
// rpcPriceGuaranteePeriod, which is defined by the price table's Expiry
func (h *Host) staticRPCUpdatePriceTable(stream siamux.Stream) error {
	// copy the host's price table and give it a random UID
	pt := h.staticPriceTables.managedCurrent()
	fastrand.Read(pt.UID[:])
	// update the epxiry to ensure prices are guaranteed for the duration of the
	// rpcPriceGuaranteePeriod
	pt.Expiry = time.Now().Add(rpcPriceGuaranteePeriod).Unix()

	// json encode the price table
	ptBytes, err := json.Marshal(pt)
	if err != nil {
		return errors.AddContext(err, "Failed to JSON encode the price table")
	}

	// send it to the renter
	uptResp := modules.RPCUpdatePriceTableResponse{PriceTableJSON: ptBytes}
	if err = modules.RPCWrite(stream, uptResp); err != nil {
		return errors.AddContext(err, "Failed to write response")
	}

	// Note that we have sent the price table before processing payment for this
	// RPC. This allows the renter to check for price gouging and close out the
	// stream if it does not agree with pricing. The price table has not yet
	// been added to the map, which means that the renter has to pay for it in
	// order for it to became active and accepted by the host.

	payment, err := h.ProcessPayment(stream)
	if err != nil {
		return errors.AddContext(err, "Failed to process payment")
	}
	if payment.Amount().Cmp(pt.UpdatePriceTableCost) < 0 {
		return modules.ErrInsufficientPaymentForRPC
	}
	// Don't expect any added collateral.
	if !payment.AddedCollateral().IsZero() {
		return fmt.Errorf("no collateral should be moved but got %v", payment.AddedCollateral().HumanString())
	}

	// after payment has been received, track the price table in the host's list
	// of price tables
	h.staticPriceTables.managedTrack(&pt)

	// refund the money we didn't use.
	refund := payment.Amount().Sub(pt.UpdatePriceTableCost)
	err = h.staticAccountManager.callRefund(payment.AccountID(), refund)
	if err != nil {
		return errors.AddContext(err, "failed to refund client")
	}
	return nil
}
