## Golang Triangle Arbitrage

### Binance and ByBit are available. Specify your exchange credentials, your personal fee, desirable order size in USDT and minimal arbitrage delta (%) in 'robot_config.toml'.

* ### Install
  1. Clone repository
  2. ```bash
     go mod tidy
     make
     ```
* ### Edit 'server_config.toml'
* ### Edit 'robot_config.toml'
* ### Run server
  ```bash
  ./execs/run_arbitrage_robot
  ```
* ### Start robot
  ```bash
  ./execs/start
  ```
* ### Update delta, lot, fee parameters
  ```bash
  ./execs/update
  ```
* ### Stop robot
  ```bash
  ./execs/stop
  ```