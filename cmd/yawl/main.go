package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"net/url"
	"strings"
	"time"

	"github.com/antchfx/htmlquery"
	"github.com/go-resty/resty/v2"
	"golang.org/x/net/html"
	"gopkg.in/yaml.v2"
)

func requestPage(client *resty.Client, url string) string {
	resp, err := client.R().
		SetHeader("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8").
		SetHeader("Cache-Control", "max-age=0").
		SetHeader("Upgrade-Insecure-Requests", "1").
		SetHeader("User-Agent", "Mozilla/5.0 (X11; Linux x86_64; rv:84.0) Gecko/20100101 Firefox/84.0").
		Get(url)
	if err != nil {
		log.Fatalf("Error fetching site: %w", err)
	}
	return string(resp.Body())
}

func processNode(
	result map[string]interface{},
	pageurl *url.URL,
	node *html.Node,
	items map[interface{}]interface{},
) {
	var (
		ok          bool
		xpath       string
		etype       string
		attribute   string
		key         interface{}
		item        interface{}
		configKey   string
		config      map[interface{}]interface{}
		subelements map[interface{}]interface{}
	)

	for key, item = range items {
		if configKey, ok = key.(string); !ok {
			log.Fatalf("config key is not a string")
		}
		if config, ok = item.(map[interface{}]interface{}); !ok {
			log.Fatalf("elements must be a map")
		}
		if xpath, ok = config["xpath"].(string); !ok {
			log.Fatalf("xpath must be a string")
		}

		if etype, ok = config["type"].(string); !ok {
			log.Fatalf("type must be a string")
		}

		switch etype {
		case "text":
			element := htmlquery.FindOne(node, xpath)
			if element != nil {
				innerText := htmlquery.InnerText(element)
				if trim, ok := config["trim"].(bool); ok && trim {
					innerText = strings.TrimSpace(innerText)
				}
				result[configKey] = innerText
			}
		case "attribute":
			if attribute, ok = config["attribute"].(string); !ok {
				log.Fatalf("attribute must be a string")
			}
			element := htmlquery.FindOne(node, xpath)
			if element != nil {
				attributeText := htmlquery.SelectAttr(element, attribute)
				if resolveUrl, ok := config["resolveUrl"].(bool); ok && resolveUrl {
					if attributeUrl, err := url.Parse(attributeText); err == nil {
						attributeText = pageurl.ResolveReference(attributeUrl).String()
					}
				}
				result[configKey] = attributeText
			}
		case "elements":
			if subelements, ok = config["elements"].(map[interface{}]interface{}); !ok {
				log.Fatalf("elements must be a map")
			}

			elements := htmlquery.Find(node, xpath)
			subresults := make([]map[string]interface{}, len(elements))
			for i, element := range elements {
				subresults[i] = make(map[string]interface{})
				processNode(subresults[i], pageurl, element, subelements)
			}
			result[configKey] = subresults
		}
	}
}

func main() {
	var (
		ok          bool
		err         error
		configData  []byte
		configKey   string
		key         interface{}
		items       interface{}
		config      map[interface{}]interface{}
		elements    map[interface{}]interface{}
		nextpage    string
		doc         *html.Node
		urlstr      string
		pageurl     *url.URL
		nextpageurl *url.URL
		page        string
		strres      []byte
	)

	configData, err = ioutil.ReadFile("config.yml")
	if err != nil {
		log.Fatalf("Config file config.yml was not found in current directory")
	}
	yaml.Unmarshal(configData, &config)

	for key, items = range config {
		if configKey, ok = key.(string); !ok {
			log.Fatalf("config key is not a string")
		}
		if config, ok = items.(map[interface{}]interface{}); !ok {
			log.Fatalf("config with key %s is not a map", configKey)
		}
		if urlstr, ok = config["url"].(string); !ok {
			log.Fatalf("Url is not a string")
		}
		if pageurl, err = url.Parse(urlstr); err != nil {
			log.Fatalf("Failed to parse url %s: %v", urlstr, err)
		}

		results := make([]interface{}, 0)

		strres, err = yaml.Marshal(config)
		fmt.Println(string(strres))

		client := resty.New()
		var rateLimiter *time.Ticker
		if requestEveryMs, ok := config["requestEveryMillis"].(int); ok {
			rateLimiter = time.NewTicker(time.Duration(requestEveryMs) * time.Millisecond)
			defer rateLimiter.Stop()
		}
		firstIteration := true
		for {
			if !firstIteration && rateLimiter != nil {
				<-rateLimiter.C
			}
			firstIteration = false

			log.Printf("Next URL: %s\n", urlstr)
			page = requestPage(client, urlstr)
			doc, err = htmlquery.Parse(strings.NewReader(page))
			if err != nil {
				log.Fatalf("Error parsing site: %w", err)
			}
			if elements, ok = config["elements"].(map[interface{}]interface{}); !ok {
				log.Fatalf("elements is not a map")
			}

			result := make(map[string]interface{})
			results = append(results, result)

			result["url"] = urlstr
			processNode(result, pageurl, doc, elements)

			if nextpage, ok = result["nextpage"].(string); !ok {
				break
			}

			// load next page
			if nextpageurl, err = url.Parse(nextpage); err != nil {
				log.Printf("Could not parse next page URL: %s", nextpage)
				break
			}
			urlstr = pageurl.ResolveReference(nextpageurl).String()
		}

		strres, err = yaml.Marshal(results)
		fmt.Println(string(strres))
	}
}
