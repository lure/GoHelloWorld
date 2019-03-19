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
		openWeatherMap{apiKey: ""},
		apixuMap{apiKey: ""},
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

	http.ListenAndServe("127.0.0.1:8080", nil)
}

func hello(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("hello, this is weather api"))
}

type weatherProvider interface {
	temperature(city string) (float64, error) // Kelvins
}
type allWeatherProvider []weatherProvider

func (w allWeatherProvider) temperature(city string, providers ...weatherProvider) (float64, error) {
	sum := 0.0
	for _, provider := range w {
		k, err := provider.temperature(city)
		if err != nil {
			return 0, err
		}
		sum += k
	}
	result := sum / float64(len(w))
	log.Printf("medium temperature: %s: %.2f", city, result)
	return result, nil
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

	defer resp.Body.Close()

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

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var msg struct {
			Error struct {
				Msg  string `json:"message"`
				Code int    `json:"code"`
			} `json:"error`
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
