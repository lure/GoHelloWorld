package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type weatherProvider interface {
	temperature(city string) (float64, error) // Kelvins
}

type allWeatherProvider []weatherProvider

//https://blog.learngoprogramming.com/
//https://golang.org/doc/effective_go.html
func main() {

	awp := allWeatherProvider{
		weatherstackMap{apiKey: "34cbb310dc26e209504e2b6f0443f33c"},
		openWeatherMap{apiKey: "742e57822bfa0a9e3310697c2d080bc8"},
	}

	http.HandleFunc("/", hello)
	http.HandleFunc("/weather/", func(w http.ResponseWriter, r *http.Request) {
		begin := time.Now()
		city := strings.SplitN(r.URL.Path, "/", 3)[2]
		data, err := awp.temperature(city)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"city":        city,
			"temperature": data,
			"took":        time.Since(begin).String(),
		})
	})

	_ = http.ListenAndServe("127.0.0.1:8080", nil)
}

func hello(w http.ResponseWriter, _ *http.Request) {
	_, _ = w.Write([]byte("hello, this is weather api"))
}

func (w allWeatherProvider) temperature(city string) (float64, error) {
	pLen := len(w)
	if pLen > 0 {
		temps := make(chan float64, pLen)
		errs := make(chan error, pLen)

		// creating go-routine and immediately apply it to current argument
		for _, provider := range w {
			go func(p weatherProvider) {
				k, err := p.temperature(city)
				if err != nil {
					errs <- err
					return
				}
				temps <- k
			}(provider)
		}

		sum := 0.0
		// select available messages, similar to non-blocking IO
		for i := 0; i < pLen; i++ {
			select {
			case temp := <-temps:
				sum += temp
			case err := <-errs:
				return 0, err
			case <-time.After(10 * time.Second):
				return 0, errors.New("timed out")
			}
		}

		result := sum / float64(pLen)
		log.Printf("medium temperature: %s: %.2f", city, result)
		return result, nil
	} else {
		log.Printf("No providers available: %s: 0", city)
		return 0, errors.New("no providers available")
	}
}

// -----------------------------------------------------------------------------
type openWeatherMap struct {
	apiKey string
}

func (w openWeatherMap) temperature(city string) (float64, error) {
	resp, err := http.Get("http://api.openweathermap.org/data/2.5/weather?APPID=" + w.apiKey + "&q=" + city)
	if err != nil {
		return 0, err
	}

	defer closeFunc(resp, "openWeatherMap")

	if resp.StatusCode != http.StatusOK {
		var msg struct {
			Msg  string `json:"message"`
			Code int    `json:"cod"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&msg)
		return 0, errors.New(strconv.Itoa(msg.Code) + ":" + msg.Msg)
	}

	var d struct {
		Name string `json:"name"`
		Main struct {
			Kelvin float64 `json:"temp"`
		} `json:"main"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&d); err != nil {
		return 0, err
	}

	log.Printf("openWeatherMap: %s: %.2f", city, d.Main.Kelvin)
	return d.Main.Kelvin, nil
}

// -----------------------------------------------------------------------------
//https://weatherstack.com/quickstart
//http://api.weatherstack.com/current?access_key=&q=Paris,tx,usa
// FIXME http://api.weatherstack.com/current?access_key=34cbb310dc26e209504e2b6f0443f33c&query=Natick
type weatherstackMap struct {
	apiKey string
}

func (w weatherstackMap) temperature(city string) (float64, error) {
	resp, err := http.Get("http://api.weatherstack.com/current?units=s&access_key=" + w.apiKey + "&query=" + city)
	if err != nil {
		return 0, err
	}

	defer closeFunc(resp, "weatherstackMap")

	if resp.StatusCode != http.StatusOK {
		var msg struct {
			Error struct {
				Msg  string `json:"info"`
				Code int    `json:"code"`
			} `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&msg)
		return 0, errors.New(strconv.Itoa(msg.Error.Code) + ":" + msg.Error.Msg)
	}

	var d struct {
		Weather struct {
			Temperature float64 `json:"temperature"`
		} `json:"current"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&d); err != nil {
		return 0, err
	}

	kelvin := d.Weather.Temperature // + 273.15
	log.Printf("weatherstack: %s: %.2f", city, kelvin)
	return kelvin, nil
}

func closeFunc(resp *http.Response, name string) {
	if err := resp.Body.Close(); err != nil {
		log.Printf(name + "failed to defer body close:" + err.Error())
	}
}
