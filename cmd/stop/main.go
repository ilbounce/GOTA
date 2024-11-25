package main

import (
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

func sendRequest(url string) (*Response, error) {

	req, err := http.NewRequest("DELETE", url, nil)
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

	url := fmt.Sprintf("http://%s:%d/robot", sConfig.Host, sConfig.Port)

	response, err := sendRequest(url)

	if err != nil {
		fmt.Println(err)
	} else {
		fmt.Println(*response)
	}

	time.Sleep(time.Second * 10)
}
