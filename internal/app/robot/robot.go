package robot

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"tarbitrage/internal/app/market"
	"tarbitrage/pkg/websocket"

	"github.com/sirupsen/logrus"
)

type Triangle struct {
	Initial market.MarketSymbol
	Middle  market.MarketSymbol
	Final   market.MarketSymbol
}

func (t *Triangle) Repr() string {
	return t.Initial.GetBaseSymbol() + "->" + t.Middle.GetBaseSymbol() + "->" + t.Final.GetBaseSymbol()
}

type Robot struct {
	Public     market.PublicClient
	Private    market.PrivateClient
	Symbols    map[string]market.MarketSymbol
	Triangles  []*Triangle
	Threashold float64
	State      *sync.Map
	Tickers    *sync.Map
	Fee        float64
	Lot        float64
	Detectors  []*Detector
	Exec       *Executor
	Quit       chan struct{}
	logger     *logrus.Logger
}

func CreateRobot(market_name, api_key, secret string, delta float64, fee float64, lot float64, logger *logrus.Logger) (*Robot, error) {

	public, err := market.NewPublicClient(market_name, logger)
	if err != nil {
		return nil, err
	}

	private, _ := market.NewPrivateClient(market_name, api_key, secret)
	executor := Executor{
		Client:  private,
		Lock:    sync.Mutex{},
		Counter: 0,
	}

	return &Robot{
		Public:     public,
		Private:    private,
		Threashold: delta,
		Quit:       make(chan struct{}),
		Symbols:    make(map[string]market.MarketSymbol),
		Triangles:  make([]*Triangle, 0),
		Tickers:    new(sync.Map),
		State:      new(sync.Map),
		Detectors:  make([]*Detector, 0),
		Exec:       &executor,
		Fee:        fee,
		Lot:        lot,
		logger:     logger,
	}, nil
}

func (r *Robot) Start() error {
	if err := r.readSymbols(); err != nil {
		return err
	}
	if err := r.readTriangles(); err != nil {
		return err
	}

	r.Exec.Fee = &r.Fee

	// pricePrecision, basePrecision, err := r.Public.GetInstrumentInfo("SANDUSDT")
	// if err != nil {
	// 	return err
	// }

	// fmt.Println(pricePrecision, basePrecision)

	request := make([]market.MarketSymbol, 0)

	for _, symbol := range r.Symbols {
		request = append(request, symbol)
	}

	if err := r.Public.GetInstrumentsInfo(request); err != nil {
		return err
	}

	fmt.Println(r.Symbols["DOT+BTC"])

	if err := r.Private.ApplyInitial(r.Lot); err != nil {
		return err
	}

	r.RunTickers()
	r.RunDetectors()

	return nil
}

func (r *Robot) RunOrderBookStream(symbol market.MarketSymbol) error {

	depthHandler := func(event *market.OrderBookEvent) {
		r.State.Store(symbol.GetBaseSymbol(), event)
	}

	errHandler := func(err error) {
		r.logger.Log(logrus.InfoLevel, err)
	}

	stream, err := r.Public.RunOrderBookStream(symbol, "5", depthHandler, errHandler)
	if err != nil {
		return err
	}

	r.Tickers.Store(symbol.GetBaseSymbol(), stream)

	return nil
}

func (r *Robot) RunTickers() error {

	wg := new(sync.WaitGroup)

	for _, symbol := range r.Symbols {
		wg.Add(1)
		go func(wg *sync.WaitGroup) {
			defer wg.Done()
			r.RunOrderBookStream(symbol)
		}(wg)
	}

	wg.Wait()

	return nil
}

func (r *Robot) StopTickers() {
	for _, symbol := range r.Symbols {
		ws, ok := r.Tickers.Load(symbol.GetBaseSymbol())
		if ok {
			ws.(*websocket.WebSocketApp).Close()
		}
	}
}

func (r *Robot) RunDetectors() {
	for _, triangle := range r.Triangles {
		d := new(Detector)
		d.Triangle = triangle
		d.Quit = make(chan struct{})
		d.Fee = r.Fee
		d.Robot = r
		d.Run()
		r.Detectors = append(r.Detectors, d)
	}
}

func (r *Robot) StopDetectors() {
	for _, detector := range r.Detectors {
		detector.Stop()
	}
}

func (r *Robot) Stop() {
	r.StopDetectors()
	r.StopTickers()
}

func (r *Robot) readSymbols() error {
	data, err := os.ReadFile(fmt.Sprintf("./files/%s/symbols.json", strings.ToLower(r.Public.Name())))
	if err != nil {
		return err
	}

	base_symbols := make([]string, 0)

	err = json.Unmarshal(data, &base_symbols)
	if err != nil {
		return err
	}

	for _, base_symbol := range base_symbols {
		r.Symbols[base_symbol] = r.Public.CreateSymbol(base_symbol)
	}

	return nil
}

func (r *Robot) readTriangles() error {
	data, err := os.ReadFile(fmt.Sprintf("./files/%s/triangles.json", strings.ToLower(r.Public.Name())))
	if err != nil {
		return err
	}

	trianlges := make([][3]string, 0)

	err = json.Unmarshal(data, &trianlges)
	if err != nil {
		return err
	}

	for _, t := range trianlges {
		triangle := Triangle{
			Initial: r.Symbols[t[0]],
			Middle:  r.Symbols[t[1]],
			Final:   r.Symbols[t[2]],
		}

		r.Triangles = append(r.Triangles, &triangle)
	}

	return nil
}

func (r *Robot) GetPrice(symbol, side string, number int) float64 {

	order_book, _ := r.State.Load(symbol)
	if order_book == nil {
		return 0.0
	}
	if len(order_book.(*market.OrderBookEvent).Asks) == 0 {
		return 0.0
	}

	switch side {
	case "ASK":
		return order_book.(*market.OrderBookEvent).Asks[number].Price
	case "BID":
		return order_book.(*market.OrderBookEvent).Bids[number].Price
	}

	return 0.0
}
