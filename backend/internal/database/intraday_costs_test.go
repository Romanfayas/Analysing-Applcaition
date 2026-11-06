package database_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/halal-equity/backend/internal/database"
)

func TestIntradayCosts_SellLegCarriesSTTAndBuyLegCarriesStamp(t *testing.T) {
	buy := database.CalculateIntradayTransactionCosts("BUY", 100000)
	sell := database.CalculateIntradayTransactionCosts("SELL", 100000)
	assert.Zero(t, buy.STT)
	assert.InDelta(t, 3.0, buy.StampDuty, 0.01)
	assert.InDelta(t, 25.0, sell.STT, 0.01, "0.025% of 1 lakh on the sell leg only")
	assert.Zero(t, sell.StampDuty)
	assert.InDelta(t, 20.0, buy.Brokerage, 0.001, "min(Rs 20, 0.03% = Rs 30)")
	assert.Zero(t, sell.DPCharges, "no depository charge for a same-day trade")
	assert.Equal(t, database.IntradayFeeModelVersion, sell.FeeModelVersion)

	small := database.CalculateIntradayTransactionCosts("BUY", 10000)
	assert.InDelta(t, 3.0, small.Brokerage, 0.001, "0.03% of 10,000 is below the Rs 20 cap")

	deliveryRound := database.CalculateDeliveryTransactionCosts("BUY", 100000).TotalFees + database.CalculateDeliveryTransactionCosts("SELL", 100000).TotalFees
	intradayRound := buy.TotalFees + sell.TotalFees
	assert.Less(t, intradayRound, deliveryRound, "same-day is cheaper in statutory charges than delivery on this size")
	assert.Zero(t, database.CalculateIntradayTransactionCosts("BUY", 0).TotalFees)
}
