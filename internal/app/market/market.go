package market

import (
	"fmt"
	"strings"
	"tarbitrage/pkg/websocket"

	"github.com/sirupsen/logrus"
)

type PublicClient interface {
	Name() string
	CreateSymbol(base_symbol string) MarketSymbol
	RunOrderBookStream(symbol MarketSymbol, levels string,
		handler OrderBookHandler, errHandler websocket.ErrHandler) (*websocket.WebSocketApp, error)
	GetInstrumentsInfo(symbols []MarketSymbol) error
}

type PrivateClient interface {
	Name() string
	GetKey() string
	GetSecret() string
	ApplyInitial(float64) error
	GetMarginBalance() (float64, error)
	PlaceOrder(symbol, side, t, quantity string) (float64, error)
}

func NewPublicClient(market string, logger *logrus.Logger) (PublicClient, error) {
	switch market {
	case "BINANCE":
		return &BinancePublicClient{
			name:   market,
			Logger: logger,
		}, nil
	case "BYBIT":
		return &BybitPublicClient{
			name:   market,
			Logger: logger,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported market: %s", market)
	}
}

func NewPrivateClient(market, api_key, secret string) (PrivateClient, error) {
	switch market {
	case "BINANCE":
		return &BinancePrivateClient{
			name:   market,
			Key:    api_key,
			Secret: secret,
		}, nil
	case "BYBIT":
		return &BybitPrivateClient{
			name:   market,
			Key:    api_key,
			Secret: secret,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported market: %s", market)
	}
}

type MarketSymbol interface {
	GetBaseAsset() string
	GetQuoteAsset() string
	GetBaseSymbol() string
	GetSymbol() string
	GetBasePrecision() int
	GetPricePrecision() int
	SetBasePrecision(int)
	SetPricePrecision(int)
}

type PriceLevel struct {
	Price    float64
	Quantity float64
}

type OrderBookEvent struct {
	Symbol string
	Asks   []PriceLevel
	Bids   []PriceLevel
}

type OrderBookHandler func(event *OrderBookEvent)

func GetPrecision(numStr string) int {
	dotIndex := strings.Index(numStr, ".")
	oneIndex := strings.Index(numStr, "1")

	if dotIndex == -1 {
		return 0
	}

	if oneIndex == 0 {
		return 0
	}

	precision := len(numStr[dotIndex:oneIndex])

	return precision
}

type SymbolPrecision struct {
	Symbol         string
	BasePrecision  int
	PricePrecision int
}
