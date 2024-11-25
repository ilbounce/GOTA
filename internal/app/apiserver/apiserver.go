package apiserver

import (
	"fmt"
	"net/http"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Host string `toml:"HOST"`
	Port int    `toml:"PORT"`
}

func readConfig(filename string) (Config, error) {
	var conf Config

	_, err := toml.DecodeFile(filename, &conf)
	if err != nil {
		return conf, err
	}

	return conf, nil
}

func Start() error {
	s := newServer()

	config, err := readConfig("./server_config.toml")
	if err != nil {
		return err
	}

	return http.ListenAndServe(fmt.Sprintf("%s:%d", config.Host, config.Port), s)
}
