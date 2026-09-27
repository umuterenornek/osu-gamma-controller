package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
)

type OSUResponse struct {
	Beatmap struct {
		Stats struct {
			AR struct {
				Converted float64 `json:"converted"`
			} `json:"ar"`
		} `json:"stats"`
	} `json:"beatmap"`
}

type ARRange struct {
	Min   float64 `json:"min"`
	Max   float64 `json:"max"`
	Value float64 `json:"value"`
}

type Config struct {
	ARMappings []ARRange `json:"ar_mappings"`
	// Backend selects how gamma is changed: "auto" (default), or on Linux "wlr", "kwin" or "x11".
	Backend string `json:"backend"`
}

type GammaManager struct {
	conn        *websocket.Conn
	done        chan struct{}
	lastARValue float64
	lastGamma   float64
	config      *Config
	gamma       GammaSetter
}

func NewGammaManager(configPath string) (*GammaManager, error) {
	config, err := loadConfig(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %v", err)
	}

	gamma, err := newGammaSetter(config.Backend)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize gamma control: %v", err)
	}
	log.Printf("Using gamma backend: %s", gamma.Name())

	return &GammaManager{
		done:        make(chan struct{}),
		lastARValue: -1,
		lastGamma:   1,
		config:      config,
		gamma:       gamma,
	}, nil
}

func loadConfig(configPath string) (*Config, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	for _, mapping := range config.ARMappings {
		if err := validateGamma(mapping.Value); err != nil {
			return nil, fmt.Errorf("AR mapping %v-%v: %v", mapping.Min, mapping.Max, err)
		}
	}

	return &config, nil
}

func (gm *GammaManager) mapARValue(arValue float64) (float64, error) {
	for _, mapping := range gm.config.ARMappings {
		if arValue >= mapping.Min && arValue <= mapping.Max {
			return mapping.Value, nil
		}
	}
	return 0, fmt.Errorf("no mapping found for AR value: %f", arValue)
}

func (gm *GammaManager) Connect() error {
	u := url.URL{Scheme: "ws", Host: "127.0.0.1:24050", Path: "/websocket/v2"}
	log.Printf("Connecting to %s", u.String())

	var err error
	gm.conn, _, err = websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		return err
	}

	log.Println("Connected to osu! websocket")
	return nil
}

func (gm *GammaManager) connectWithRetry() {
	for {
		select {
		case <-gm.done:
			return
		default:
			if err := gm.Connect(); err != nil {
				log.Printf("Failed to connect to websocket: %v. Retrying in 10 seconds...", err)
				time.Sleep(10 * time.Second)
			} else {
				return
			}
		}
	}
}

func (gm *GammaManager) Start() {
	go func() {
		for {
			select {
			case <-gm.done:
				return
			default:
				if gm.conn == nil {
					log.Println("No websocket connection available, attempting to reconnect...")
					gm.connectWithRetry()
					if gm.conn == nil {
						time.Sleep(2 * time.Second)
						continue
					}
				}
				gm.readAndProcess()
			}
		}
	}()
}

func (gm *GammaManager) readAndProcess() {
	gm.conn.SetReadDeadline(time.Now().Add(30 * time.Second))
	_, message, err := gm.conn.ReadMessage()
	if err != nil {
		log.Printf("Error reading from websocket: %v", err)
		log.Println("Resetting gamma to 1 due to connection loss...")
		gm.adjustGamma(1)
		gm.conn.Close()
		gm.conn = nil
		gm.lastARValue = -1
		log.Println("Connection lost, will attempt to reconnect...")
		return
	}

	var response OSUResponse
	if err := json.Unmarshal(message, &response); err != nil {
		log.Printf("Error parsing JSON response: %v", err)
		return
	}

	arValue := response.Beatmap.Stats.AR.Converted
	if arValue == gm.lastARValue {
		return
	}
	log.Printf("AR value changed: %f -> %f", gm.lastARValue, arValue)
	gm.lastARValue = arValue

	mappedValue, err := gm.mapARValue(arValue)
	if err != nil {
		log.Printf("Error mapping AR value: %v", err)
		return
	}
	log.Printf("Mapped AR value %f to gamma value %f", arValue, mappedValue)
	gm.adjustGamma(mappedValue)
}

func (gm *GammaManager) adjustGamma(mappedValue float64) {
	if mappedValue == gm.lastGamma {
		return
	}
	if err := gm.gamma.Set(mappedValue); err != nil {
		log.Printf("Error changing gamma: %v", err)
		return
	}
	gm.lastGamma = mappedValue
	log.Printf("Successfully changed gamma to %f", mappedValue)
}

func (gm *GammaManager) Stop() {
	close(gm.done)
	if gm.conn != nil {
		gm.conn.Close()
	}

	log.Println("Restoring original gamma...")
	if err := gm.gamma.Close(); err != nil {
		log.Printf("Error restoring gamma: %v", err)
	}
}

func main() {
	configPath := "config.json"
	if len(os.Args) > 1 {
		configPath = os.Args[1]
	}

	gm, err := NewGammaManager(configPath)
	if err != nil {
		log.Fatalf("Failed to create gamma manager: %v", err)
	}

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	gm.Start()
	log.Printf("Gamma manager started with config: %s. Will attempt to connect to websocket and retry every 10 seconds if connection fails. Press Ctrl+C to stop.", configPath)

	<-c
	log.Println("Shutting down...")

	gm.Stop()
	log.Println("Gamma manager stopped.")
}
