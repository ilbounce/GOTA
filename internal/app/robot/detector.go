package robot

import (
	"fmt"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

type Detector struct {
	Triangle *Triangle
	Fee      float64
	Quit     chan struct{}
	Robot    *Robot
}

type Detection struct {
	bbs, ssb float64
}

func arbitrage_formula(a, b, c, fee float64, sequence string) float64 {
	var x float64
	switch sequence {
	case "BBS":
		x = c/(a*b) - (fee * 3 / 100.0)
	case "SSB":
		x = 2.0 - c/(a*b) - (fee * 3 / 100.0)
	}

	return x
}

func (d *Detector) get_price(symbol, side string, number int) float64 {
	return d.Robot.GetPrice(symbol, side, number)
}

func (d *Detector) bbs(det *Detection, wg *sync.WaitGroup) {
	defer wg.Done()
	a := d.get_price(d.Triangle.Initial.GetBaseSymbol(), "ASK", 0)
	b := d.get_price(d.Triangle.Middle.GetBaseSymbol(), "ASK", 0)
	c := d.get_price(d.Triangle.Final.GetBaseSymbol(), "BID", 0)

	if a == 0.0 || b == 0.0 || c == 0.0 {
		det.bbs = 0.0
		return
	}

	det.bbs = arbitrage_formula(a, b, c, d.Fee, "BBS")
}

func (d *Detector) ssb(det *Detection, wg *sync.WaitGroup) {
	defer wg.Done()
	a := d.get_price(d.Triangle.Initial.GetBaseSymbol(), "BID", 0)
	b := d.get_price(d.Triangle.Middle.GetBaseSymbol(), "BID", 0)
	c := d.get_price(d.Triangle.Final.GetBaseSymbol(), "ASK", 0)

	if a == 0.0 || b == 0.0 || c == 0.0 {
		det.ssb = 0.0
		return
	}

	det.bbs = arbitrage_formula(a, b, c, d.Fee, "SSB")
}

func (d *Detector) Run() {
	go func() {
		possibility := 0.0
		ticker := time.NewTicker(time.Millisecond * 100)
		defer ticker.Stop()
		for {
			select {
			case <-d.Quit:
				d.Robot.logger.Log(logrus.InfoLevel, d.Triangle.Repr(), "is stopped.")
				return
			case <-ticker.C:
				detection := new(Detection)
				wg := new(sync.WaitGroup)
				wg.Add(2)
				go d.bbs(detection, wg)
				go d.ssb(detection, wg)
				wg.Wait()

				if detection.bbs > 1.0+d.Robot.Threashold {
					cur := (detection.bbs - 1.0) * 100
					if possibility != cur {
						possibility = cur
						d.Robot.logger.Log(logrus.InfoLevel,
							fmt.Sprintf("Find arbitrage possibility %s (Buy, Buy, Sell), Percent: %.2f\n", d.Triangle.Repr(), possibility))
						if d.Robot.Exec.Counter >= 3 {
							continue
						}
						if err := d.Robot.Exec.ExecuteTriangle(d.Triangle, "BBS", d.Robot.Lot); err != nil {
							d.Robot.logger.Log(logrus.InfoLevel, err)
						}
					}
				} else if detection.ssb > 1.0+d.Robot.Threashold {
					cur := (detection.bbs - 1.0) * 100
					if possibility != cur {
						possibility = cur
						d.Robot.logger.Log(logrus.InfoLevel,
							fmt.Sprintf("Find arbitrage possibility %s (Sell, Sell, Buy), Percent: %.2f\n", d.Triangle.Repr(), possibility))
						if d.Robot.Exec.Counter >= 3 {
							continue
						}
						if err := d.Robot.Exec.ExecuteTriangle(d.Triangle, "SSB", d.Robot.Lot); err != nil {
							d.Robot.logger.Log(logrus.InfoLevel, err)
						}
					}
				} else {
					possibility = 0.0
				}
			}
		}
	}()
}

func (d *Detector) Stop() {
	d.Quit <- struct{}{}
}
