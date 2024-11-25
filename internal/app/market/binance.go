package market

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"tarbitrage/pkg/websocket"
	"time"

	"github.com/sirupsen/logrus"
)

var BinanceHost string = "https://api.binance.com"
var BinanceBaseWsUrl string = "wss://stream.binance.com:9443/ws"

var BinanceHeaders = map[string]string{
	"Accept":     "application/json",
	"User-Agent": "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/56.0.2924.87 Safari/537.36",
}

type BinancePublicClient struct {
	name   string
	Logger *logrus.Logger
}

type BinancePrivateClient struct {
	name   string
	Key    string
	Secret string
	Logger *logrus.Logger
}

type BinanceSymbol struct {
	BaseAsset      string
	QuoteAsset     string
	BaseSymbol     string
	Symbol         string
	BasePrecision  int
	PricePrecision int
}

var BinanceSides = map[string]string{
	"BUY":  "BUY",
	"SELL": "SELL",
}

func (s *BinanceSymbol) GetBaseAsset() string {
	return s.BaseAsset
}

func (s *BinanceSymbol) GetQuoteAsset() string {
	return s.QuoteAsset
}

func (s *BinanceSymbol) GetBaseSymbol() string {
	return s.BaseSymbol
}

func (s *BinanceSymbol) GetSymbol() string {
	return s.Symbol
}

func (s *BinanceSymbol) GetBasePrecision() int {
	return s.BasePrecision
}

func (s *BinanceSymbol) GetPricePrecision() int {
	return s.PricePrecision
}

func (s *BinanceSymbol) SetBasePrecision(prec int) {
	s.BasePrecision = prec
}

func (s *BinanceSymbol) SetPricePrecision(prec int) {
	s.PricePrecision = prec
}

func (c *BinancePublicClient) Name() string {
	return c.name
}

func (c *BinancePrivateClient) Name() string {
	return c.name
}

func (c *BinancePrivateClient) GetKey() string {
	return c.Key
}

func (c *BinancePrivateClient) GetSecret() string {
	return c.Secret
}

func (c *BinancePublicClient) CreateSymbol(base_symbol string) MarketSymbol {
	assets := strings.Split(base_symbol, "+")
	return &BinanceSymbol{
		BaseAsset:  assets[0],
		QuoteAsset: assets[1],
		Symbol:     assets[0] + assets[1],
		BaseSymbol: base_symbol,
	}
}

func (c *BinancePublicClient) RunOrderBookStream(symbol MarketSymbol, levels string, handler OrderBookHandler,
	errHandler websocket.ErrHandler) (*websocket.WebSocketApp, error) {

	endpoint := BinanceBaseWsUrl
	wsApp := new(websocket.WebSocketApp)

	type event struct {
		Bids [][]string `json:"bids"`
		Asks [][]string `json:"asks"`
	}
	wsApp.OnOpen = func(ws *websocket.WebSocketApp) {
		subscription := map[string]interface{}{
			"method": "SUBSCRIBE",
			"params": []string{
				fmt.Sprintf("%s@depth%s@100ms", strings.ToLower(symbol.GetSymbol()), levels),
			},
			"id": time.Now().UnixMilli(),
		}
		ws.Send(subscription)
		c.Logger.Log(logrus.InfoLevel, fmt.Sprintf("Stream %s %s has been started.\n", c.Name(), symbol.GetBaseSymbol()))
	}
	wsApp.OnMessage = func(ws *websocket.WebSocketApp, message []byte) {
		e := new(event)
		err := json.Unmarshal(message, e)

		if err != nil {
			errHandler(err)
			return
		}

		if len(e.Asks) == 0 {
			return
		}

		book := new(OrderBookEvent)

		book.Asks = make([]PriceLevel, len(e.Asks))
		book.Bids = make([]PriceLevel, len(e.Bids))

		for i := 0; i < len(book.Asks); i++ {
			item := e.Asks[i]
			price, _ := strconv.ParseFloat(item[0], 64)
			quantity, _ := strconv.ParseFloat(item[1], 64)
			book.Asks[i] = PriceLevel{
				Price:    price,
				Quantity: quantity,
			}
		}

		for i := 0; i < len(book.Bids); i++ {
			item := e.Bids[i]
			price, _ := strconv.ParseFloat(item[0], 64)
			quantity, _ := strconv.ParseFloat(item[1], 64)
			book.Bids[i] = PriceLevel{
				Price:    price,
				Quantity: quantity,
			}
		}

		handler(book)
	}
	wsApp.OnError = func(ws *websocket.WebSocketApp, err error) {
		c.Logger.Log(logrus.InfoLevel, err)
	}
	wsApp.OnClose = func(ws *websocket.WebSocketApp) {
		c.Logger.Log(logrus.InfoLevel, fmt.Sprintf("Stream %s %s is stopped.\n", c.Name(), symbol.GetBaseSymbol()))
		if ws.IsRunning {
			ws.Run(ws.Config.Endpoint, ws.Config.WebsocketKeepalive, ws.Config.Timeout)
		}
	}

	wsApp.OnPing = func(ws *websocket.WebSocketApp, ping []byte) {
		wsApp.SendPong(ping)
	}

	if err := wsApp.Run(endpoint, false, 0); err != nil {
		return nil, err
	}

	return wsApp, nil
}

func (c *BinancePublicClient) Perform(query map[string]interface{}, method string, session_method string, result interface{}) error {
	keys := make([]string, 0, len(query))
	for k := range query {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Create a new map with sorted keys
	sortedQuery := make(map[string]interface{})
	for _, k := range keys {
		sortedQuery[k] = query[k]
	}

	var queryStrings []string
	for k, v := range sortedQuery {
		if v != nil {
			queryStrings = append(queryStrings, fmt.Sprintf("%s=%v", k, v))
		}
	}
	queryString := strings.Join(queryStrings, "&")

	url := fmt.Sprintf("%s/%s?%s", BinanceHost, method, queryString)

	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest(session_method, url, nil)

	if err != nil {
		return err
	}

	for key, value := range BinanceHeaders {
		req.Header.Set(key, value)
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	res, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	err = json.Unmarshal(res, &result)
	if err != nil {
		return err
	}

	return nil
}

func (c *BinancePublicClient) GetInstrumentsInfo(symbols []MarketSymbol) error {

	request := make([]string, len(symbols))

	for idx, s := range symbols {
		request[idx] = fmt.Sprintf("\"%s\"", s.GetSymbol())
	}

	query := "[" + strings.Join(request, ",") + "]"

	parameters := map[string]interface{}{
		"symbols": query,
	}

	type filter struct {
		Type     string `json:"filterType"`
		TickSize string `json:"tickSize"`
		StepSize string `json:"stepSize"`
	}

	type SymbolData struct {
		Symbol  string
		Filters []filter `json:"filters"`
	}

	type response struct {
		Code    int          `json:"code"`
		Message string       `json:"msg"`
		List    []SymbolData `json:"symbols"`
	}

	resp := new(response)
	if err := c.Perform(parameters, "api/v3/exchangeInfo", "GET", resp); err != nil {
		return err
	}

	if resp.Code != 0 {
		return fmt.Errorf("binance error: code: %d, message: %s", resp.Code, resp.Message)
	}

	findSymbol := func(data []SymbolData, symbol string) (SymbolData, bool) {
		for _, elm := range data {
			if elm.Symbol == symbol {
				return elm, true
			}
		}
		return SymbolData{}, false
	}

	res := resp.List

	for _, s := range symbols {
		data, ok := findSymbol(res, s.GetSymbol())
		if !ok {
			return fmt.Errorf("can't find precision information for symbol %s", s.GetBaseSymbol())
		}
		for _, f := range data.Filters {
			switch f.Type {
			case "PRICE_FILTER":
				s.SetPricePrecision(GetPrecision(f.TickSize))
			case "LOT_SIZE":
				s.SetBasePrecision(GetPrecision(f.StepSize))
			}
		}
	}

	return nil
}

func (c *BinancePrivateClient) generateSignature(queryString string) string {
	h := hmac.New(sha256.New, []byte(c.Secret))
	h.Write([]byte(queryString))
	signature := hex.EncodeToString(h.Sum(nil))
	return signature
}

func (c *BinancePrivateClient) PerformSign(query map[string]interface{}, method string, session_method string, result interface{}) error {
	keys := make([]string, 0, len(query))
	for k := range query {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Create a new map with sorted keys
	sortedQuery := make(map[string]interface{})
	for _, k := range keys {
		sortedQuery[k] = query[k]
	}

	var queryStrings []string
	for k, v := range sortedQuery {
		if v != nil {
			queryStrings = append(queryStrings, fmt.Sprintf("%s=%v", k, v))
		}
	}
	queryString := strings.Join(queryStrings, "&")

	sortedQuery["signature"] = c.generateSignature(queryString)

	queryStrings = make([]string, 0)
	for k, v := range sortedQuery {
		queryStrings = append(queryStrings, fmt.Sprintf("%s=%v", k, v))
	}

	payload := strings.Join(queryStrings, "&")

	url := fmt.Sprintf("%s/%s?%s", BinanceHost, method, payload)
	BinanceHeaders["X-MBX-APIKEY"] = c.Key

	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest(session_method, url, nil)

	if err != nil {
		return err
	}

	for key, value := range BinanceHeaders {
		req.Header.Set(key, value)
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	res, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	err = json.Unmarshal(res, &result)
	if err != nil {
		return err
	}

	return nil
}

func (c *BinancePrivateClient) ApplyInitial(lot float64) error {
	balance, err := c.GetMarginBalance()
	if err != nil {
		return err
	}
	if balance < lot {
		return fmt.Errorf("you have not enough balance (should be greater than lot)")
	}
	return nil
}

func (c *BinancePrivateClient) GetMarginBalance() (float64, error) {
	parameters := map[string]interface{}{
		"timestamp": time.Now().UnixMilli(),
	}

	type result struct {
		Value   string `json:"totalCollateralValueInUSDT"`
		Code    int    `json:"code"`
		Message string `json:"msg"`
	}

	resp := new(result)

	if err := c.PerformSign(parameters, "/sapi/v1/margin/account", "GET", resp); err != nil {
		return 0.0, err
	}
	if resp.Code != 0 {
		return 0.0, fmt.Errorf("binance error: code: %d, message: %s", resp.Code, resp.Message)
	}
	available, _ := strconv.ParseFloat(resp.Value, 64)

	return available, nil
}

func (c *BinancePrivateClient) PlaceOrder(symbol, side, t, quantity string) (float64, error) {
	parameters := map[string]interface{}{
		"symbol":           symbol,
		"side":             BinanceSides[side],
		"type":             "MARKET",
		"timestamp":        time.Now().UnixMilli(),
		"newClientOrderId": strconv.FormatInt(time.Now().UnixMilli(), 10),
		"isIsolated":       "False",
	}

	switch t {
	case "open":
		parameters["quoteOrderQty"] = quantity
		parameters["sideEffectType"] = "MARGIN_BUY"
	case "close":
		parameters["quantity"] = quantity
		parameters["sideEffectType"] = "AUTO_REPAY"
	}

	type Fill struct {
		Price      string `json:"price"`
		Quantity   string `json:"qty"`
		Commission string `json:"commission"`
	}

	type response struct {
		Code          int    `json:"code"`
		Message       string `json:"msg"`
		Price         string `json:"price"`
		Quantity      string `json:"executedQty"`
		Side          string `json:"side"`
		QuoteQuantity string `json:"cummulativeQuoteQty"`
		Fills         []Fill `json:"fills"`
	}

	resp := new(response)

	if err := c.PerformSign(parameters, "/sapi/v1/margin/order", "POST", resp); err != nil {
		return 0, err
	}
	if resp.Code != 0 {
		return 0, fmt.Errorf("binance error: code: %d, message: %s", resp.Code, resp.Message)
	}

	qty, _ := strconv.ParseFloat(resp.Quantity, 64)

	return qty, nil
}
