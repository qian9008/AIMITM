package config

import (
	"encoding/json"
	"os"
	"sync"
	"sync/atomic"
)

var ConfigUpdateChan = make(chan *Config, 1)

type RewriteRule struct {
	Path   string `json:"path"`
	Action string `json:"action"` // REPLACE, REMOVE, ADD
	Value  string `json:"value,omitempty"`
}

type HeaderRule struct {
	Name   string `json:"name"`
	Action string `json:"action"` // SET, REMOVE
	Value  string `json:"value,omitempty"`
}

type UpstreamGroup struct {
	Name        string        `json:"name"`
	BaseURL     string        `json:"base_url"`
	Hosts       []string      `json:"hosts"`
	Rewrites    []RewriteRule `json:"rewrites"`
	Headers     []HeaderRule  `json:"headers"`
	IsRemainder bool          `json:"is_remainder"`
}

type Config struct {
	ProxyPort   int             `json:"proxy_port"`
	AdminPort   int             `json:"admin_port"`
	Debug       bool            `json:"debug"`
	RedirectAll bool            `json:"redirect_all"`
	MITMHosts   []string        `json:"mitm_hosts"`
	Upstreams   []UpstreamGroup `json:"upstreams"`
}

var (
	configValue atomic.Value
	configLock  sync.Mutex
	configPath  = "config.json"
)

func Init() error {
	var cfg Config
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		cfg = Config{
			ProxyPort:   8080,
			AdminPort:   8888,
			Debug:       false,
			RedirectAll: false,
			MITMHosts:   []string{"*.openai.com"},
			Upstreams:   []UpstreamGroup{},
		}
		configValue.Store(&cfg)
		return Save()
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return err
	}

	if err := json.Unmarshal(data, &cfg); err != nil {
		return err
	}
	configValue.Store(&cfg)
	return nil
}

func Get() *Config {
	val := configValue.Load()
	if val == nil {
		return &Config{}
	}
	return val.(*Config)
}

func Update(newConfig Config) error {
	configLock.Lock()
	defer configLock.Unlock()
	configValue.Store(&newConfig)
	select {
	case ConfigUpdateChan <- &newConfig:
	default:
	}
	return Save()
}

func Save() error {
	cfg := Get()
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(configPath, data, 0644)
}
