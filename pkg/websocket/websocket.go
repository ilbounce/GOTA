package websocket

import (
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

// WsHandler handle raw websocket message
type WsHandler func(message []byte)

// ErrHandler handles errors
type ErrHandler func(err error)

type OnOpenHandler func(ws *WebSocketApp)
type OnMessageHandler func(ws *WebSocketApp, message []byte)
type OnErrorHandler func(ws *WebSocketApp, err error)
type OnCloseHandler func(ws *WebSocketApp)
type OnPongHandler func(ws *WebSocketApp, pong string)
type OnPingHandler func(ws *WebSocketApp, ping []byte)

type WebSocketApp struct {
	OnOpen     OnOpenHandler
	OnMessage  OnMessageHandler
	OnError    OnErrorHandler
	OnClose    OnCloseHandler
	OnPong     OnPongHandler
	OnPing     OnPingHandler
	Config     *WsConfig
	Connection *websocket.Conn
	Stop       chan struct{}
	Done       chan struct{}
	IsRunning  bool
}

func (ws *WebSocketApp) connect() (*websocket.Conn, error) {
	proxy := http.ProxyFromEnvironment
	Dialer := websocket.Dialer{
		Proxy:             proxy,
		HandshakeTimeout:  45 * time.Second,
		EnableCompression: false,
	}

	c, _, err := Dialer.Dial(ws.Config.Endpoint, nil)

	return c, err
}

func (ws *WebSocketApp) keepAlive() {
	ticker := time.NewTicker(ws.Config.Timeout)

	lastResponse := time.Now()
	ws.Connection.SetPongHandler(func(msg string) error {
		lastResponse = time.Now()
		if ws.OnPong != nil {
			ws.OnPong(ws, msg)
		}
		return nil
	})

	go func() {
		defer ticker.Stop()
		for {
			deadline := time.Now().Add(10 * time.Second)
			err := ws.Connection.WriteControl(websocket.PingMessage, []byte{}, deadline)
			if err != nil {
				return
			}
			<-ticker.C
			if time.Since(lastResponse) > ws.Config.Timeout {
				ws.Connection.Close()
				return
			}
		}
	}()
}

func (ws *WebSocketApp) Run(endpoint string, keep_alive bool, timeout time.Duration) error {
	ws.Config = NewWsConfig(endpoint, keep_alive, timeout)
	conn, err := ws.connect()
	if err != nil {
		return err
	}
	ws.Connection = conn
	if ws.OnOpen != nil {
		ws.OnOpen(ws)
	}

	ws.Connection.SetReadLimit(655350)
	ws.Done = make(chan struct{})
	ws.Stop = make(chan struct{})

	ws.IsRunning = true

	go func() {
		// This function will exit either on error from
		// websocket.Conn.ReadMessage or when the stopC channel is
		// closed by the client.
		defer close(ws.Done)
		defer ws.Connection.Close()
		if ws.OnClose != nil {
			defer ws.OnClose(ws)
		}
		if ws.Config.WebsocketKeepalive {
			ws.keepAlive()
		}
		// Wait for the stopC channel to be closed.  We do that in a
		// separate goroutine because ReadMessage is a blocking
		// operation.
		silent := false
		go func() {
			select {
			case <-ws.Stop:
				silent = true
				ws.Connection.Close()
			case <-ws.Done:
			}
		}()
		for {
			op, message, err := ws.Connection.ReadMessage()
			if err != nil {
				if !silent {
					if ws.OnError != nil {
						ws.OnError(ws, err)
					}
				}
				return
			}

			if op == 9 {
				if ws.OnPing != nil {
					ws.OnPing(ws, message)
				}
			} else {
				if ws.OnMessage != nil {
					ws.OnMessage(ws, message)
				}
			}
		}
	}()

	return nil
}

func (ws *WebSocketApp) Send(message interface{}) error {
	err := ws.Connection.WriteJSON(message)

	return err
}

func (ws *WebSocketApp) SendPong(pong []byte) error {
	err := ws.Connection.WriteMessage(10, pong)

	return err
}

func (ws *WebSocketApp) Close() {
	ws.IsRunning = false
	ws.Stop <- struct{}{}
	<-ws.Done
}

// WsConfig webservice configuration
type WsConfig struct {
	Endpoint           string
	WebsocketKeepalive bool
	Timeout            time.Duration
}

func NewWsConfig(endpoint string, keep_alive bool, timeout time.Duration) *WsConfig {
	return &WsConfig{
		Endpoint:           endpoint,
		WebsocketKeepalive: keep_alive,
		Timeout:            timeout,
	}
}
