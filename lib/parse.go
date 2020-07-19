package gobot

import (
	"io"
	"log"
	"net/url"
	"sync"

	"golang.org/x/net/html"
)

// LinkNode ...
type LinkNode struct {
	Link   string `json:"link"`
	Status bool   `json:"status"`
}

func closer(c io.Closer) {
	err := c.Close()
	if err != nil {
		log.Printf("Error: %+v", err)
	}
}

// Replace mutex with atomic switch for perf
var mux sync.Mutex

func sendTokens(tokenizer *html.Tokenizer) chan html.Token {
	out := make(chan html.Token, 10)
	go func() {
		for notEnd := true; notEnd; {
			mux.Lock()
			tokenType := tokenizer.Next()
			mux.Unlock()
			switch tokenType {
			case html.ErrorToken:
				notEnd = false
			case html.StartTagToken:
				token := tokenizer.Token()
				out <- token
			}
		}
		close(out)
	}()
	return out
}

func sendAttributes(in chan html.Token) chan html.Attribute {
	out := make(chan html.Attribute, 10)
	go func() {
		for token := range in {
			for _, attr := range token.Attr {
				out <- attr
			}
		}
		close(out)
	}()
	return out
}

func filterAttributes(in chan html.Attribute, filterFn func(html.Attribute) bool) chan html.Attribute {
	out := make(chan html.Attribute, 10)
	go func() {
		for attribute := range in {
			if filterFn(attribute) {
				out <- attribute
			}
		}
		close(out)
	}()
	return out
}

func filterTokens(in chan html.Token, filterFn func(html.Token) bool) chan html.Token {
	out := make(chan html.Token, 10)
	go func() {
		for token := range in {
			if filterFn(token) {
				out <- token
			}
		}
		close(out)
	}()
	return out
}

func parseAttributes(body io.Reader, tag string, attribute string) chan string {
	tokenizer := html.NewTokenizer(body)
	tokenChan := sendTokens(tokenizer)
	tagChan := filterTokens(tokenChan, func(t html.Token) bool {
		return t.Data == tag
	})
	attributeChan := filterAttributes(sendAttributes(tagChan), func(a html.Attribute) bool {
		if a.Key == attribute {
			link, err := url.ParseRequestURI(a.Val)
			if link == nil || err != nil {
				return false
			}
			if link.Scheme != "" {
				return true
			}
		}
		return false
	})
	linkChan := make(chan string, 10)
	go func() {
		for attr := range attributeChan {
			linkChan <- attr.Val
		}
		close(linkChan)
	}()
	return linkChan
}

func startParsing(body io.Reader, numJobs int) <-chan string {
	var wg sync.WaitGroup
	out := make(chan string, 10)
	merge := func(in chan string) {
		for i := range in {
			out <- i
		}
		wg.Done()
	}
	wg.Add(numJobs)
	for i := 0; i < numJobs; i++ {
		go merge(parseAttributes(body, "a", "href"))
	}
	go func() {
		wg.Wait()
		close(out)
	}()
	return out
}

// GetLinks returns a map that contains the links as keys and their statuses as values
func GetLinks(rootLink string) ([]LinkNode, error) {
	// Creating new Tor connection
	client := newDualClient(&ClientConfig{timeout: defaultTimeout})
	resp, err := client.Get(rootLink)
	if err != nil {
		return nil, err
	}
	defer closer(resp.Body)
	linkChan := startParsing(resp.Body, 16)
	linkCollection := make([]LinkNode, 0)
	for link := range startConverting(linkChan, client, 16) {
		linkCollection = append(linkCollection, link)
	}
	return linkCollection, nil
}
