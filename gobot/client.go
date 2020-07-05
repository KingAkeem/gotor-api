package gobot

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	defaultHost    = "127.0.0.1"
	defaultPort    = "9050"
	defaultTimeout = 600
)

// Client represents a GoBot HTTP client
type Client interface {
	Get(string) (*http.Response, error)
	Head(string) (*http.Response, error)
}

// ClientConfig contains configuration for a client
type ClientConfig struct {
	addr    string
	port    string
	timeout int
}

type dualClient struct {
	regClient *http.Client
	torClient *http.Client
}

func (d *dualClient) Head(link string) (*http.Response, error) {
	if strings.Contains(link, ".onion") {
		return d.torClient.Head(link)
	}

	return d.regClient.Head(link)
}

func (d *dualClient) Get(link string) (*http.Response, error) {
	if strings.Contains(link, ".onion") {
		return d.torClient.Get(link)
	}

	return d.regClient.Get(link)
}

// NewDualClient creates an HTTP client capable of performing TOR requests.
func newDualClient(config *ClientConfig) *dualClient {
	if config.addr == "" {
		config.addr = defaultHost
	}

	if config.port == "" {
		config.port = defaultPort
	}

	torProxyURL, err := url.Parse(fmt.Sprintf("socks5://%s:%s", config.addr, config.port))
	if err != nil {
		log.Fatal("Unable to parse Tor Proxy URL. Error:", err)
	}
	torTransport := &http.Transport{Proxy: http.ProxyURL(torProxyURL)}

	return &dualClient{
		regClient: &http.Client{Timeout: time.Second * time.Duration(config.timeout)},
		torClient: &http.Client{Transport: torTransport, Timeout: time.Second * time.Duration(config.timeout)},
	}
}
