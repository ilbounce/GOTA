package apiserver

import (
	"encoding/json"
	"fmt"
	"tarbitrage/internal/app/robot"

	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"

	"net/http"
)

type server struct {
	router       *mux.Router
	logger       *logrus.Logger
	bot          *robot.Robot
	inProcess    bool
	botIsRunning bool
}

func newServer() *server {
	s := &server{
		router: mux.NewRouter(),
		logger: logrus.New(),
	}

	s.configureRouter()

	return s
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.router.ServeHTTP(w, r)
}

func (s *server) configureRouter() {
	s.router.HandleFunc("/robot", s.handleStartRobot()).Methods("POST")
	s.router.HandleFunc("/robot", s.handleStopRobot()).Methods("DELETE")
	s.router.HandleFunc("/robot", s.handleUpdateRobot()).Methods("PUT")
}

func (s *server) handleStartRobot() http.HandlerFunc {
	type request struct {
		Delta   float64 `json:"delta"`
		Market  string  `json:"market"`
		API_KEY string  `json:"api_key"`
		Secret  string  `json:"secret"`
		Fee     float64 `json:"fee"`
		Lot     float64 `json:"lot"`
	}

	return func(w http.ResponseWriter, r *http.Request) {
		defer func() { s.inProcess = false }()
		if s.inProcess {
			s.raiseError(w, http.StatusBadRequest, fmt.Errorf("service is busy"))
			return
		}

		if s.botIsRunning {
			s.raiseError(w, http.StatusBadRequest, fmt.Errorf("robot has been already launched"))
			return
		}

		s.inProcess = true
		req := &request{}

		if err := json.NewDecoder(r.Body).Decode(req); err != nil {
			s.raiseError(w, http.StatusBadRequest, err)
			return
		}

		bot, err := robot.CreateRobot(req.Market, req.API_KEY, req.Secret, req.Delta/100.0, req.Fee, req.Lot, s.logger)
		if err != nil {
			s.raiseError(w, http.StatusBadRequest, err)
			return
		}

		if err := bot.Start(); err != nil {
			s.raiseError(w, http.StatusBadRequest, err)
			return
		}

		s.bot = bot
		s.botIsRunning = true

		s.logger.Log(logrus.InfoLevel, fmt.Sprintf("Robot started; Exchange: %s; Trading lot: %.2f.", s.bot.Public.Name(), s.bot.Lot))

		s.respond(w, http.StatusCreated, struct {
			Status string `json:"status"`
		}{
			Status: "ok",
		})

	}
}

func (s *server) handleUpdateRobot() http.HandlerFunc {
	type request struct {
		Delta   float64 `json:"delta"`
		Market  string  `json:"market"`
		API_KEY string  `json:"api_key"`
		Secret  string  `json:"secret"`
		Lot     float64 `json:"lot"`
		Fee     float64 `json:"fee"`
	}

	return func(w http.ResponseWriter, r *http.Request) {
		req := &request{}

		if err := json.NewDecoder(r.Body).Decode(req); err != nil {
			s.raiseError(w, http.StatusBadRequest, err)
			return
		}

		if !s.botIsRunning {
			s.raiseError(w, http.StatusBadRequest, fmt.Errorf("robot is not running"))
		}

		if req.Market != s.bot.Public.Name() || req.API_KEY != s.bot.Private.GetKey() || req.Secret != s.bot.Private.GetSecret() {
			s.raiseError(w, http.StatusBadRequest, fmt.Errorf("you can update either delta, lot or fee"))
		}

		if req.Delta == s.bot.Threashold && req.Lot == s.bot.Lot && req.Fee == s.bot.Fee {
			s.raiseError(w, http.StatusBadRequest, fmt.Errorf("no new parameters in request"))
		}

		s.bot.Threashold = req.Delta
		s.bot.Lot = req.Lot
		s.bot.Fee = req.Fee

		s.logger.Log(logrus.InfoLevel, fmt.Sprintf("Robot updated; Exchange: %s; Lot: %.2f; Delta: %.4f; Fee: %.2f",
			s.bot.Public.Name(), s.bot.Lot, s.bot.Threashold, s.bot.Fee))

		s.respond(w, http.StatusCreated, struct {
			Status string `json:"status"`
		}{
			Status: "ok",
		})
	}
}

func (s *server) handleStopRobot() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defer func() { s.inProcess = false }()
		if s.inProcess {
			s.raiseError(w, http.StatusBadRequest, fmt.Errorf("service is busy"))
			return
		}

		if !s.botIsRunning {
			s.raiseError(w, http.StatusBadRequest, fmt.Errorf("robot is not running"))
			return
		}

		s.inProcess = true
		s.bot.Stop()
		s.botIsRunning = false

		s.logger.Log(logrus.InfoLevel, fmt.Sprintf("Robot stopped; Exchange: %s.", s.bot.Public.Name()))

		s.respond(w, http.StatusCreated, struct {
			Status string `json:"status"`
		}{
			Status: "ok",
		})
	}
}

func (s *server) raiseError(w http.ResponseWriter, code int, err error) {
	s.respond(w, code, map[string]string{"error": err.Error()})
}

func (s *server) respond(w http.ResponseWriter, code int, data interface{}) {
	w.WriteHeader(code)
	if data != nil {
		json.NewEncoder(w).Encode(data)
	}
}
