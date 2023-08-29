package feeds

import (
	"context"
	"fmt"
	"math/rand"
	"net/http"
	"net/url"
	"strings"

	"github.com/gorilla/feeds"
	"github.com/labstack/echo/v4"
)

const (
	paramNameFormat      = "format"
	paramNameBrands      = "brands"
	paramNameCategoryIDs = "categorieIds"
	paramNameOutletIDs   = "outletIds"
	paramNameKeyword     = "text"

	formatRSS  = "rss"
	formatAtom = "atom"
	formatJSON = "json"

	feedBaseURL     = "https://api.grube.fund/"
	feedCacheMaxAge = 60 * 60
)

type Generator interface {
	BuildFeed(ctx context.Context, brands []string, categoryIDs []string, outletIDs []string, keyword string) (*feeds.Feed, error)
}

type Handler struct {
	generator Generator
}

func NewHandler(generator Generator) *Handler {
	return &Handler{
		generator: generator,
	}
}

func (h Handler) HandleGet(c echo.Context) error {
	ctx := c.Request().Context()

	format := c.Param(paramNameFormat)
	if format != formatRSS && format != formatAtom && format != formatJSON {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid format")
	}

	brands, err := parseRequiredBrands(c.QueryParam(paramNameBrands))
	if err != nil {
		return err
	}

	// Use "misspelled" category id param name to match params of official website
	categoryIDs, err := parseRequiredCategoryIDs(c.QueryParam(paramNameCategoryIDs))
	if err != nil {
		return err
	}

	outletIDs := parseOptionalOutletIDs(c.QueryParam(paramNameOutletIDs))
	keyword := c.QueryParam(paramNameKeyword)

	feed, err := h.generator.BuildFeed(ctx, brands, categoryIDs, outletIDs, keyword)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError).SetInternal(err)
	}

	// Set link to URL of request
	feedURL, err := buildFeedURL(*c.Request().URL)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError).SetInternal(err)
	}
	feed.Link = &feeds.Link{Href: feedURL}

	var contentType, content string
	if format == formatRSS {
		contentType = echo.MIMEApplicationXMLCharsetUTF8
		content, err = feed.ToRss()
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError).SetInternal(err)
		}
	} else if format == formatAtom {
		contentType = echo.MIMEApplicationXML
		content, err = feed.ToAtom()
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError).SetInternal(err)
		}
	} else if format == formatJSON {
		contentType = echo.MIMEApplicationJSON
		content, err = feed.ToJSON()
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError).SetInternal(err)
		}
	} else {
		return echo.NewHTTPError(http.StatusNotFound)
	}

	c.Response().Header().Set(echo.HeaderContentType, contentType)
	c.Response().Header().Set(echo.HeaderCacheControl, fmt.Sprintf("max-age=%d, must-revalidate", randomMaxAge()))
	return c.String(http.StatusOK, content)
}

func parseRequiredBrands(brands string) ([]string, error) {
	if brands == "" {
		return nil, echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("%s param is required", paramNameBrands))
	}

	return strings.Split(brands, ","), nil
}

func parseRequiredCategoryIDs(categoryIDs string) ([]string, error) {
	if categoryIDs == "" {
		return nil, echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("%s param is required", paramNameCategoryIDs))
	}

	return strings.Split(categoryIDs, ","), nil
}

func parseOptionalOutletIDs(outletIDs string) []string {
	return strings.Split(outletIDs, ",")
}

func buildFeedURL(requestURL url.URL) (string, error) {
	u, err := url.Parse(feedBaseURL)
	if err != nil {
		return "", err
	}
	u = u.JoinPath(requestURL.EscapedPath())
	u.RawQuery = requestURL.RawQuery
	return u.String(), nil
}

// randomMaxAge Calculate a random max age (+/- 15 minutes) to avoid many items expiring in similar intervals
func randomMaxAge() int {
	offset := rand.Intn(15*60 + 1)
	if rand.Intn(2) == 1 {
		return feedCacheMaxAge - offset
	}
	return feedCacheMaxAge + offset
}
