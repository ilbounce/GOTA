package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/BurntSushi/toml"
)

type ServerConfig struct {
	Host string `toml:"HOST"`
	Port int    `toml:"PORT"`
}

type RobotConfig struct {
	Market string  `toml:"MARKET"`
	Key    string  `toml:"API_KEY"`
	Secret string  `toml:"SECRET"`
	Delta  float64 `toml:"DELTA"`
	Lot    float64 `toml:"LOT"`
	Fee    float64 `toml:"FEE"`
}

type RequestData struct {
	Market string  `json:"market"`
	Key    string  `json:"api_key"`
	Secret string  `json:"secret"`
	Delta  float64 `json:"delta"`
	Lot    float64 `json:"lot"`
	Fee    float64 `json:"fee"`
}

type Response struct {
	StatusCode   int
	Status       string `json:"status"`
	ErrorMessage string `json:"error"`
}

func readServerConfig(filename string) (ServerConfig, error) {
	var conf ServerConfig

	_, err := toml.DecodeFile(filename, &conf)
	if err != nil {
		return conf, err
	}

	return conf, nil
}

func readRobotConfig(filename string) (RobotConfig, error) {
	var conf RobotConfig

	_, err := toml.DecodeFile(filename, &conf)
	if err != nil {
		return conf, err
	}

	return conf, nil
}

func sendRequest(url string, data RequestData) (*Response, error) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	res, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	result := new(Response)
	err = json.Unmarshal(res, &result)
	if err != nil {
		return nil, err
	}

	result.StatusCode = resp.StatusCode

	return result, nil
}

func main() {

	sConfig, err := readServerConfig("./server_config.toml")
	if err != nil {
		log.Fatal(err)
	}
	rConfig, err := readRobotConfig("./robot_config.toml")
	if err != nil {
		log.Fatal(err)
	}

	data := RequestData{
		Market: rConfig.Market,
		Key:    rConfig.Key,
		Secret: rConfig.Secret,
		Delta:  rConfig.Delta,
		Lot:    rConfig.Lot,
		Fee:    rConfig.Fee,
	}

	url := fmt.Sprintf("http://%s:%d/robot", sConfig.Host, sConfig.Port)

	response, err := sendRequest(url, data)

	if err != nil {
		fmt.Println(err)
	} else {
		fmt.Println(*response)
	}

	time.Sleep(time.Second * 10)
}
