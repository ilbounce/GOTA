package robot

import (
	"strconv"
	"sync"
	"tarbitrage/internal/app/market"
)

type Executor struct {
	Client  market.PrivateClient
	Lock    sync.Mutex
	Counter int
	Fee     *float64
}

func (ex *Executor) ExecuteTriangle(triangle *Triangle, sequence string, lot float64) error {
	ex.Counter++
	fee := *(ex.Fee)
	defer func() { ex.Counter-- }()

	var rollback [2]string
	switch sequence {
	case "BBS":
		quantity := strconv.FormatFloat(lot, 'f', triangle.Initial.GetPricePrecision(), 64)
		ex.Lock.Lock()
		qty, err := ex.Client.PlaceOrder(triangle.Initial.GetSymbol(), "BUY", "open", quantity)
		ex.Lock.Unlock()
		if err != nil {
			return err
		}
		quantity = strconv.FormatFloat(qty, 'f', triangle.Middle.GetPricePrecision(), 64)
		rollback[0] = quantity
		ex.Lock.Lock()
		qty, err = ex.Client.PlaceOrder(triangle.Middle.GetSymbol(), "BUY", "open", quantity)
		ex.Lock.Unlock()
		if err != nil {
			ex.Client.PlaceOrder(triangle.Initial.GetSymbol(), "SELL", "close", rollback[0])
			return err
		}
		quantity = strconv.FormatFloat(qty, 'f', triangle.Middle.GetBasePrecision(), 64)
		rollback[1] = quantity
		ex.Lock.Lock()
		_, err = ex.Client.PlaceOrder(triangle.Final.GetSymbol(), "SELL", "close", quantity)
		ex.Lock.Unlock()
		if err != nil {
			ex.Client.PlaceOrder(triangle.Middle.GetSymbol(), "SELL", "close", rollback[1])
			ex.Client.PlaceOrder(triangle.Initial.GetSymbol(), "SELL", "close", rollback[0])
			return err
		}
	case "SSB":
		quantity := strconv.FormatFloat(lot, 'f', triangle.Initial.GetPricePrecision(), 64)
		ex.Lock.Lock()
		qty, err := ex.Client.PlaceOrder(triangle.Initial.GetSymbol(), "SELL", "open", quantity)
		ex.Lock.Unlock()
		if err != nil {
			return err
		}
		qty = qty * (1 + fee/100)
		quantity = strconv.FormatFloat(qty, 'f', triangle.Middle.GetPricePrecision(), 64)
		rollback[0] = quantity
		ex.Lock.Lock()
		qty, err = ex.Client.PlaceOrder(triangle.Middle.GetSymbol(), "SELL", "open", quantity)
		ex.Lock.Unlock()
		if err != nil {
			ex.Client.PlaceOrder(triangle.Initial.GetSymbol(), "BUY", "close", rollback[0])
			return err
		}
		qty = qty * (1 + fee/100)
		quantity = strconv.FormatFloat(qty, 'f', triangle.Middle.GetBasePrecision(), 64)
		rollback[1] = quantity
		ex.Lock.Lock()
		_, err = ex.Client.PlaceOrder(triangle.Final.GetSymbol(), "BUY", "close", quantity)
		ex.Lock.Unlock()
		if err != nil {
			ex.Client.PlaceOrder(triangle.Middle.GetSymbol(), "BUY", "close", rollback[1])
			ex.Client.PlaceOrder(triangle.Initial.GetSymbol(), "BUY", "close", rollback[0])
			return err
		}
	}

	return nil
}
