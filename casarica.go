package main

import (
	"errors"
	"github.com/PuerkitoBio/goquery"
	"github.com/headzoo/surf"
	"github.com/headzoo/surf/browser"
	log "github.com/sirupsen/logrus"
	"regexp"
	"strconv"
	"strings"
)

const(
	CRStartURL = "https://www.casarica.com.py/"
)

var(
	errSKUMatch = errors.New("No se pudo obtener SKU")
)

type CasaRicaScraper struct {
	browser *browser.Browser
	productBrowser *browser.Browser
	ids map[int]int

	skuExpr *regexp.Regexp
}

func (s *CasaRicaScraper) Init() error {
	s.browser = surf.NewBrowser()
	err := s.browser.Open(CRStartURL)
	if err != nil {
		return err
	}
	s.ids = make(map[int]int)
	s.skuExpr = regexp.MustCompile(`"sku": "(.*?)"`)
	return nil
}

func(s *CasaRicaScraper) getSKU(p *Product) (error) {
	if s.productBrowser == nil {
		s.productBrowser = surf.NewBrowser()
	}
	err := s.browser.Open(p.URL)
	if err != nil {
		return err
	}
	head, err :=  s.browser.Find("head").Html()
	if err != nil {
		return err
	}
	occurs := s.skuExpr.FindAllStringSubmatch(head, -1)
	if len(occurs) == 0 {
		return errSKUMatch
	}
	submatch := occurs[0]
	if len(submatch) < 2 {
		return errSKUMatch
	}
	p.SKU = strings.TrimSpace(submatch[1])
	return nil
}

func (s *CasaRicaScraper) getProductPrice(priceElem *goquery.Selection) (int, bool, error) {
	text := strings.TrimSpace(priceElem.Text())
	if text == "" {
		return -1, false, errors.New("Empty price element text")
	}
	price := strings.ToLower(text)
	price = strings.Replace(price, "gs", "", -1)
	var perKg bool
	if strings.Contains(price, "kg.") {
		perKg = true
		price = strings.Replace(price, "kg.", "", -1)
		price = strings.Replace(price, "el", "", -1)
	}
	price = strings.Replace(price, ".", "", -1)
	price = strings.Replace(price, " ", "", -1)
	n, err := strconv.Atoi(price)
	return n, perKg, err
}

func (s *CasaRicaScraper) navigate() (relHref string, keepBrowsing bool) {
	pagination := s.browser.Find(".pagination")
	if pagination.Size() > 0 {
		pagination.Find("li a").Each(func(_ int, pagElem *goquery.Selection) {
			rel, found := pagElem.Attr("rel")
			if !found {
			}
			if strings.Contains(rel, "next") {
				relHref, _ = pagElem.Attr("href")
				keepBrowsing = true
				return
			}
		})
	}
	return relHref, keepBrowsing
}

func (s *CasaRicaScraper) Fetch(productFn func(*Product)) {
	s.browser.Find("#sideNavbar a").Each(func(_ int, catElem *goquery.Selection) {
		catURL, _ := catElem.Attr("href")
		if !strings.Contains(catURL, "https://") {
			return
		}
		// Ignorar promociones por ahora:
		if strings.Contains(catURL, "promociones") {
			return
		}
		err := s.browser.Open(catURL)
		if err != nil {
			panic(err)
		}
		for {
			s.browser.Find(".divproduct").Each(func(_ int, productDiv *goquery.Selection) {
				pidElem := productDiv.Find(".productsListId")
				productIDStr, found := pidElem.Attr("value")
				if !found {
					log.Error("Ignorando producto (no se pudo obtener el ID de producto)", catURL)
					return
				}
				productID, err := strconv.Atoi(productIDStr)
				if err != nil {
					log.Error("Ignorando producto (error al convertir ID de producto)", catURL)
					return
				}
				if _, found := s.ids[productID]; found {
					log.Println("Repetido", productID)
					return
				}
				s.ids[productID] = 0
				// pTitleElem := productDiv.Find(".ptitle")
				pSubtitleElem := productDiv.Find(".psubtitle")
				title := strings.TrimSpace(pSubtitleElem.Text())
				priceElem := productDiv.Find(".pprice")
				price, perKg, err := s.getProductPrice(priceElem)
				if err != nil {
					log.Error("Ignorando producto (no se pudo obtener el precio)", "catURL=", catURL, "productID=", productID, "title=", title)
					return
				}
				productHrefElem := productDiv.Find(".pimg a")
				productHref, exists := productHrefElem.Attr("href")
				if !exists {
					log.Error("Ignorando producto (no se pudo obtener el enlace del producto)", "catURL=", catURL, "productID=", productID, "title=", title)
					return
				}
				product := &Product{
					ID:          productID,
					Name:        title,
					URL:         productHref,
					Price:       price,
					CategoryID:  0,
					CategoryURL: catURL,
					PerKg:       perKg,
				}
				go func(p *Product) {
					err = s.getSKU(product)
					if err != nil {
						log.Error(err)
					}
					productFn(product)
				}(product)
			})
			nextHref, keepBrowsing := s.navigate()
			if !keepBrowsing {
				break
			}
			s.browser.Open(nextHref)
		}
	})
	log.Println("Finalizado")
}
