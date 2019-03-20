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

func main() {

	awp := allWeatherProvider{
		apixuMap{apiKey: ""},
		openWeatherMap{apiKey: ""},
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
		json.NewEncoder(w).Encode(map[string]interface{}{
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

type weatherProvider interface {
	temperature(city string) (float64, error) // Kelvins
}
type allWeatherProvider []weatherProvider

func (w allWeatherProvider) temperature(city string, providers ...weatherProvider) (float64, error) {
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
		json.NewDecoder(resp.Body).Decode(&msg)
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
//https://api.apixu.com/v1/current.json?key=&q=Paris,tx,usa
type apixuMap struct {
	apiKey string
}

func (w apixuMap) temperature(city string) (float64, error) {
	resp, err := http.Get("https://api.apixu.com/v1/current.json?key=" + w.apiKey + "&q=" + city)
	if err != nil {
		return 0, err
	}

	defer closeFunc(resp, "apixuMap")

	if resp.StatusCode != http.StatusOK {
		var msg struct {
			Error struct {
				Msg  string `json:"message"`
				Code int    `json:"code"`
			} `json:"error"`
		}
		json.NewDecoder(resp.Body).Decode(&msg)
		return 0, errors.New(strconv.Itoa(msg.Error.Code) + ":" + msg.Error.Msg)
	}

	var d struct {
		Current struct {
			Celsius float64 `json:"temp_c"`
		} `json:"current"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&d); err != nil {
		return 0, err
	}

	kelvin := d.Current.Celsius + 273.15
	log.Printf("apixu: %s: %.2f", city, kelvin)
	return kelvin, nil
}

func closeFunc(resp *http.Response, name string) {
	if err := resp.Body.Close(); err != nil {
		log.Printf(name + "failed to defer body close:" + err.Error())
	}
}
