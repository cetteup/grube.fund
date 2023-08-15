package feed

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/feeds"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

const (
	timeout = 5 * time.Second

	perPage   = 100
	userAgent = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/114.0.0.0 Safari/537.36"
)

type postingsAPIResponse struct {
	Postings []posting `json:"postings"`
	HasMore  bool      `json:"morePostingsAvailable"`
}

type posting struct {
	ID           string  `json:"posting_id"`
	Text         string  `json:"posting_text"`
	ProductName  string  `json:"name"`
	ProductID    int     `json:"pim_id"`
	CategoryID   string  `json:"top_level_catalog_id"`
	Price        string  `json:"price"`
	ShippingCost float64 `json:"shipping_cost"`
	Brand        brand   `json:"brand"`
	Outlet       outlet  `json:"outlet"`
}

func (p posting) BuildWebURL(baseURI string) (string, error) {
	u, err := url.Parse(baseURI)
	if err != nil {
		return "", err
	}

	q := u.Query()
	q.Add("outletIds", strconv.Itoa(p.Outlet.ID))
	q.Add("brands", p.Brand.Name)
	q.Add("categorieIds", p.CategoryID)
	q.Add("text", strconv.Itoa(p.ProductID))
	u.RawQuery = q.Encode()

	return u.String(), nil
}

func (p posting) ToFeedItem(baseURI string) (*feeds.Item, error) {
	webURL, err := p.BuildWebURL(baseURI)
	if err != nil {
		return nil, err
	}

	price, err := strconv.ParseFloat(p.Price, 64)
	if err != nil {
		return nil, err
	}

	return &feeds.Item{
		Title:   formatItemTitle(p.ProductName, price, p.ShippingCost),
		Link:    &feeds.Link{Href: webURL},
		Id:      p.ID,
		Content: p.Text,
	}, nil
}

type brand struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type outlet struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type Generator struct {
	store      string
	apiBaseURI string
	webBaseURI string
	client     *http.Client
}

func NewGenerator(store string, apiBaseURI string, webBaseURI string) *Generator {
	return &Generator{
		store:      store,
		apiBaseURI: apiBaseURI,
		webBaseURI: webBaseURI,
		client: &http.Client{
			Timeout: timeout,
		},
	}
}

func (g *Generator) BuildFeed(ctx context.Context, brands []string, categoryIDs []string, keyword string) (*feeds.Feed, error) {
	postings, err := g.fetch(ctx, brands, categoryIDs, keyword)
	if err != nil {
		return nil, err
	}

	feed := &feeds.Feed{
		Title: fmt.Sprintf("Fundgrube Artikel von %s", g.store),
		Author: &feeds.Author{
			Name:  "grube.fund",
			Email: "feed@grube.fund",
		},
		Subtitle: formatFeedSubtitle(brands, categoryIDs, keyword),
		Created:  time.Now(),
	}

	for _, p := range postings {
		var item *feeds.Item
		item, err = p.ToFeedItem(g.webBaseURI)
		if err != nil {
			return nil, err
		}

		feed.Items = append(feed.Items, item)
	}

	return feed, nil
}

func (g *Generator) fetch(ctx context.Context, brands []string, categoryIDs []string, keyword string) ([]posting, error) {
	postings := make([]posting, 0)
	offset := 0
	hasMore := true
	for hasMore {
		req, err := g.buildRequest(ctx, perPage, offset, brands, categoryIDs, keyword)
		if err != nil {
			return nil, err
		}

		resp, err := g.client.Do(req)
		if err != nil {
			return nil, err
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}
		err = resp.Body.Close()
		if err != nil {
			return nil, err
		}

		if resp.StatusCode != http.StatusOK {
			// API returns 422 when offset exceeds max value (seems to max return 990 postings)
			if resp.StatusCode == http.StatusUnprocessableEntity {
				return postings, nil
			}
			return nil, fmt.Errorf("failed to fetch postings, received unexpected status code: %d (%s)", resp.StatusCode, resp.Status)
		}

		var r postingsAPIResponse
		err = json.Unmarshal(body, &r)
		if err != nil {
			return nil, err
		}

		postings = append(postings, r.Postings...)
		hasMore = r.HasMore
		offset += perPage
	}

	return postings, nil
}

func (g *Generator) buildRequest(ctx context.Context, limit int, offset int, brands []string, categoryIDs []string, keyword string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, g.apiBaseURI, nil)
	if err != nil {
		return nil, err
	}

	q := req.URL.Query()
	q.Add("limit", strconv.Itoa(limit))
	q.Add("offset", strconv.Itoa(offset))
	q.Add("brands", strings.Join(brands, ","))
	q.Add("categorieIds", strings.Join(categoryIDs, ","))
	q.Add("text", keyword)
	req.URL.RawQuery = q.Encode()

	req.Header.Set("User-Agent", userAgent)

	return req, nil
}

func formatItemTitle(productName string, price float64, shippingCost float64) string {
	printer := message.NewPrinter(language.German)

	var shippingCostStr string
	if shippingCost == 0.0 {
		shippingCostStr = "kostenlos"
	} else {
		shippingCostStr = printer.Sprintf("%.2f€", shippingCost)
	}

	return printer.Sprintf("%s - %.2f€ (Versand: %s)", productName, price, shippingCostStr)
}

func formatFeedSubtitle(brands []string, categoryIDs []string, keyword string) string {
	subtitle := fmt.Sprintf("Marken: %s/Kategorien: %s", strings.Join(brands, ", "), strings.Join(categoryIDs, ", "))
	if keyword != "" {
		subtitle += fmt.Sprintf("/Stichwort: %s", keyword)
	}
	return subtitle
}
