// Package source reads funds and stock prices from the public endpoints of
// 天天基金 and 腾讯财经.
package source

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"golang.org/x/text/encoding/simplifiedchinese"

	"fundbar/internal/model"
)

// Client asks the endpoints. It is safe for concurrent use.
type Client struct {
	HTTP *http.Client
}

func New() *Client {
	return &Client{HTTP: &http.Client{Timeout: 20 * time.Second}}
}

var errNotFound = errors.New("source: not found")

// Fund is what one request to the fund site tells about a fund.
type Fund struct {
	Code string
	Name string
	// Navs is the fund's net asset value each day, oldest first.
	Navs []model.Nav
	// StockPosition is the share of the fund's assets that stocks take, in
	// percent, as the latest report says.
	StockPosition float64
	// Symbols are the stocks in the fund's latest report, as "sh600519".
	Symbols []string
	// Industries is the fund's allocation by industry in its latest report,
	// heaviest first.
	Industries []model.Industry
	// IndustryReport is the date of that report.
	IndustryReport string
	// IndustriesLoaded tells a cache without industries from one whose
	// report simply holds none.
	IndustriesLoaded bool
}

func (c *Client) get(ctx context.Context, url, referer string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Referer", referer)
	req.Header.Set("Accept", "*/*")
	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: %s", url, res.Status)
	}
	return io.ReadAll(io.LimitReader(res.Body, 32<<20))
}

const userAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Safari/605.1.15"

// Fund reads a fund's name, net asset value history and holdings from its
// page's data script.
func (c *Client) Fund(ctx context.Context, code string) (*Fund, error) {
	url := "https://fund.eastmoney.com/pingzhongdata/" + code + ".js"
	body, err := c.get(ctx, url, "https://fund.eastmoney.com/"+code+".html")
	if err != nil {
		return nil, err
	}
	src := string(body)
	if !strings.Contains(src, "Data_netWorthTrend") {
		return nil, errNotFound
	}
	f := &Fund{Code: code, Name: jsString(src, "fS_name")}
	if f.Name == "" {
		return nil, errNotFound
	}

	if raw := jsArray(src, "Data_netWorthTrend"); raw != "" {
		var rows []struct {
			X            int64   `json:"x"`
			Y            float64 `json:"y"`
			EquityReturn float64 `json:"equityReturn"`
		}
		if err := json.Unmarshal([]byte(raw), &rows); err != nil {
			return nil, fmt.Errorf("net worth: %w", err)
		}
		for _, r := range rows {
			if r.Y <= 0 || r.X <= 0 {
				continue
			}
			f.Navs = append(f.Navs, model.Nav{
				Date: time.UnixMilli(r.X).Format("2006-01-02"),
				Unit: r.Y,
				Pct:  r.EquityReturn,
			})
		}
		// 累计净值, which the same script holds as [[ms, value], …].
		if raw := jsArray(src, "Data_ACWorthTrend"); raw != "" {
			var rows [][]float64
			if json.Unmarshal([]byte(raw), &rows) == nil {
				acc := make(map[string]float64, len(rows))
				for _, r := range rows {
					if len(r) >= 2 && r[0] > 0 {
						acc[time.UnixMilli(int64(r[0])).Format("2006-01-02")] = r[1]
					}
				}
				for i := range f.Navs {
					f.Navs[i].Acc = acc[f.Navs[i].Date]
				}
			}
		}
	}
	if len(f.Navs) == 0 {
		return nil, errNotFound
	}

	if raw := jsArray(src, "Data_fundSharesPositions"); raw != "" {
		var rows [][]float64
		if json.Unmarshal([]byte(raw), &rows) == nil && len(rows) > 0 {
			f.StockPosition = rows[len(rows)-1][1]
		}
	}
	if raw := jsArray(src, "stockCodesNew"); raw != "" {
		var codes []string
		if json.Unmarshal([]byte(raw), &codes) == nil {
			for _, c := range codes {
				if s := Symbol(c); s != "" {
					f.Symbols = append(f.Symbols, s)
				}
			}
		}
	}
	return f, nil
}

// Holdings reads the stocks of a fund's latest quarterly report, from the
// page of its 持仓.
func (c *Client) Holdings(ctx context.Context, code string) ([]model.Holding, string, error) {
	url := "https://fundf10.eastmoney.com/FundArchivesDatas.aspx?type=jjcc&code=" + code + "&topline=10"
	body, err := c.get(ctx, url, "https://fundf10.eastmoney.com/ccmx_"+code+".html")
	if err != nil {
		return nil, "", err
	}
	src := string(body)
	// Only the newest report: the tables of the older ones follow it.
	if i := strings.Index(src, "</table>"); i > 0 {
		src = src[:i]
	}
	var report string
	if i := strings.LastIndex(src, "截止至："); i >= 0 {
		rest := src[i+len("截止至："):]
		if j := strings.Index(rest, "</font>"); j >= 0 {
			rest = rest[:j]
		}
		report = cleanTag(rest)
	}
	holdings := parseHoldings(src)
	if len(holdings) == 0 {
		return nil, report, errNotFound
	}
	return holdings, report, nil
}

// Industries reads the fund's allocation by industry in its latest report,
// heaviest first.
func (c *Client) Industries(ctx context.Context, code string) ([]model.Industry, string, error) {
	url := "https://api.fund.eastmoney.com/f10/HYPZ/?fundCode=" + code + "&year="
	body, err := c.get(ctx, url, "https://fundf10.eastmoney.com/hytz_"+code+".html")
	if err != nil {
		return nil, "", err
	}
	var res struct {
		Data struct {
			QuarterInfos []struct {
				Report string `json:"JZRQ"`
				Rows   []struct {
					Name   string `json:"HYMC"`
					Weight string `json:"ZJZBL"`
				} `json:"HYPZInfo"`
			} `json:"QuarterInfos"`
		} `json:"Data"`
	}
	if err := json.Unmarshal(body, &res); err != nil {
		return nil, "", fmt.Errorf("industries: %w", err)
	}
	if len(res.Data.QuarterInfos) == 0 {
		return nil, "", nil
	}
	latest := res.Data.QuarterInfos[0]
	industries := make([]model.Industry, 0, len(latest.Rows))
	for _, row := range latest.Rows {
		name := strings.TrimSpace(row.Name)
		weight, err := strconv.ParseFloat(strings.TrimSpace(row.Weight), 64)
		if name == "" || err != nil || weight <= 0 {
			continue
		}
		industries = append(industries, model.Industry{Name: name, Weight: weight})
	}
	if len(industries) == 0 {
		return nil, latest.Report, nil
	}
	sort.SliceStable(industries, func(i, j int) bool {
		return industries[i].Weight > industries[j].Weight
	})
	return industries, latest.Report, nil
}

// Profiles reads the industry, board and themes of stocks, in one request
// per batch.
func (c *Client) Profiles(ctx context.Context, symbols []string) (map[string]model.StockProfile, error) {
	out := make(map[string]model.StockProfile, len(symbols))
	const chunk = 60
	for start := 0; start < len(symbols); start += chunk {
		end := min(start+chunk, len(symbols))
		if err := c.profilesChunk(ctx, symbols[start:end], out); err != nil {
			return out, err
		}
	}
	// A stock the endpoint knows nothing about still has a place, so that
	// it is not asked for again every refresh.
	for _, symbol := range symbols {
		if _, ok := out[symbol]; !ok {
			out[symbol] = model.StockProfile{}
		}
	}
	return out, nil
}

func (c *Client) profilesChunk(ctx context.Context, symbols []string, out map[string]model.StockProfile) error {
	var secids []string
	byCode := make(map[string]string, len(symbols))
	for _, symbol := range symbols {
		sid, ok := secID(symbol)
		if !ok {
			continue
		}
		secids = append(secids, sid)
		byCode[symbol[2:]] = symbol
	}
	if len(secids) == 0 {
		return nil
	}
	url := "https://push2.eastmoney.com/api/qt/ulist.np/get?secids=" +
		strings.Join(secids, ",") + "&fields=f12,f100,f102,f103"
	body, err := c.get(ctx, url, "https://quote.eastmoney.com/")
	if err != nil {
		return err
	}
	var res struct {
		Data struct {
			Rows []struct {
				Code     string `json:"f12"`
				Industry string `json:"f100"`
				Board    string `json:"f102"`
				Concepts string `json:"f103"`
			} `json:"diff"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &res); err != nil {
		return fmt.Errorf("profiles: %w", err)
	}
	for _, row := range res.Data.Rows {
		symbol := byCode[row.Code]
		if symbol == "" {
			continue
		}
		out[symbol] = model.StockProfile{
			Industry: strings.TrimSpace(row.Industry),
			Board:    strings.TrimSpace(row.Board),
			Concepts: splitConcepts(row.Concepts),
		}
	}
	return nil
}

// secID turns "sh600519" into Eastmoney's "1.600519".
func secID(symbol string) (string, bool) {
	if len(symbol) < 3 {
		return "", false
	}
	switch symbol[:2] {
	case "sh":
		return "1." + symbol[2:], true
	case "sz":
		return "0." + symbol[2:], true
	case "hk":
		return "116." + symbol[2:], true
	case "us":
		return "105." + symbol[2:], true
	}
	return "", false
}

func splitConcepts(s string) []string {
	parts := strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || r == '，' || r == ';' || r == '；'
	})
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

func parseHoldings(src string) []model.Holding {
	var out []model.Holding
	for _, row := range strings.Split(src, "<tr") {
		link := ""
		if i := strings.Index(row, "unify/r/"); i >= 0 {
			link = row[i+len("unify/r/"):]
			if j := strings.IndexAny(link, "'\""); j >= 0 {
				link = link[:j]
			}
		}
		// A stock whose exchange has no price here still shows, without a
		// symbol, as the holdings of a fund that buys abroad do.
		symbol := Symbol(link)
		var name string
		if i := strings.Index(row, "<td class='tol'>"); i >= 0 {
			name = cellName(row[i+len("<td class='tol'>"):])
		}
		var weight float64
		for _, cell := range strings.Split(row, "<td class='tor'>")[1:] {
			text := strings.TrimSpace(cleanTag(cell))
			if strings.HasSuffix(text, "%") {
				weight, _ = strconv.ParseFloat(strings.TrimSuffix(text, "%"), 64)
				break
			}
		}
		if weight <= 0 {
			continue
		}
		_, code, _ := strings.Cut(link, ".")
		out = append(out, model.Holding{
			Symbol: symbol,
			Code:   code,
			Name:   name,
			Weight: weight,
		})
	}
	return out
}

// cellName takes the name out of a cell: the text of its first link, or the
// cell's own text when it has none.
func cellName(cell string) string {
	if i := strings.Index(cell, "<a"); i >= 0 {
		rest := cell[i:]
		if j := strings.IndexByte(rest, '>'); j >= 0 {
			rest = rest[j+1:]
			if k := strings.Index(rest, "</a>"); k >= 0 {
				if name := strings.TrimSpace(rest[:k]); name != "" {
					return name
				}
			}
		}
	}
	return cleanTag(cell)
}

// cleanTag takes the text out of the first cell or link of a fragment.
func cleanTag(s string) string {
	var b strings.Builder
	depth := 0
	for _, r := range s {
		switch r {
		case '<':
			depth++
		case '>':
			if depth > 0 {
				depth--
			}
		default:
			if depth == 0 {
				b.WriteRune(r)
			}
		}
	}
	out := strings.TrimSpace(b.String())
	if i := strings.IndexAny(out, "&"); i >= 0 {
		out = out[:i]
	}
	return strings.TrimSpace(out)
}

// Symbol turns "1.600519" into "sh600519", and "116.00700" into "hk00700".
func Symbol(dotted string) string {
	market, code, ok := strings.Cut(strings.TrimSpace(dotted), ".")
	if !ok {
		return ""
	}
	if code == "" {
		return ""
	}
	switch market {
	case "1":
		return "sh" + code
	case "0":
		return "sz" + code
	case "116", "128", "113", "124":
		return "hk" + code
	case "105", "106", "107":
		return "us" + code
	}
	return ""
}

// Search asks the fund site for funds matching a word or a code.
func (c *Client) Search(ctx context.Context, key string) ([]Hit, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return nil, nil
	}
	url := "https://fundsuggest.eastmoney.com/FundSearch/api/FundSearchAPI.ashx?m=1&key=" +
		strings.NewReplacer(" ", "%20").Replace(key)
	body, err := c.get(ctx, url, "https://fund.eastmoney.com/")
	if err != nil {
		return nil, err
	}
	hits, err := parseSearch(body)
	if err != nil {
		return nil, err
	}
	return hits, nil
}

// parseSearch reads the answer of the search, which holds funds, indices and
// stocks alike.
func parseSearch(body []byte) ([]Hit, error) {
	var res struct {
		Datas []struct {
			Code         string `json:"CODE"`
			Name         string `json:"NAME"`
			CategoryDesc string `json:"CATEGORYDESC"`
			// The rest of a fund's entry holds numbers of more than one
			// type, which a search does not need.
			FundBaseInfo struct {
				Type string `json:"FTYPE"`
			} `json:"FundBaseInfo"`
		} `json:"Datas"`
	}
	if err := json.Unmarshal(body, &res); err != nil {
		return nil, err
	}
	hits := make([]Hit, 0, len(res.Datas))
	for _, d := range res.Datas {
		// Indices and stocks answer too: only funds have a value to show.
		if d.Code == "" || d.Name == "" || d.CategoryDesc != "基金" {
			continue
		}
		hits = append(hits, Hit{Code: d.Code, Name: d.Name, Type: d.FundBaseInfo.Type})
	}
	return hits, nil
}

// Hit is a fund a search found.
type Hit struct {
	Code string
	Name string
	Type string
}

// Quotes reads the prices of stocks now, in one request.
func (c *Client) Quotes(ctx context.Context, symbols []string) (map[string]model.Quote, error) {
	out := make(map[string]model.Quote, len(symbols))
	const chunk = 60
	for start := 0; start < len(symbols); start += chunk {
		end := min(start+chunk, len(symbols))
		if err := c.quotesChunk(ctx, symbols[start:end], out); err != nil {
			return out, err
		}
	}
	return out, nil
}

func (c *Client) quotesChunk(ctx context.Context, symbols []string, out map[string]model.Quote) error {
	if len(symbols) == 0 {
		return nil
	}
	url := "https://qt.gtimg.cn/q=" + strings.Join(symbols, ",")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Referer", "https://gu.qq.com/")
	res, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("quotes: %s", res.Status)
	}
	raw, err := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if err != nil {
		return err
	}
	text, err := simplifiedchinese.GBK.NewDecoder().Bytes(raw)
	if err != nil {
		text = raw
	}
	for _, part := range strings.Split(string(text), ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		name, value, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		symbol := strings.TrimPrefix(strings.TrimSpace(name), "v_")
		fields := strings.Split(strings.Trim(strings.TrimSpace(value), `"`), "~")
		if len(fields) < 33 {
			continue
		}
		price, err1 := strconv.ParseFloat(fields[3], 64)
		pct, err2 := strconv.ParseFloat(fields[32], 64)
		if err1 != nil || err2 != nil || price <= 0 {
			continue
		}
		q := model.Quote{Symbol: symbol, Price: price, Pct: pct}
		if at := parseQuoteTime(fields[30]); !at.IsZero() {
			q.At = at
		}
		out[symbol] = q
	}
	return nil
}

func parseQuoteTime(s string) time.Time {
	for _, layout := range []string{"20060102150405", "2006/01/02 15:04:05"} {
		if t, err := time.ParseInLocation(layout, strings.TrimSpace(s), time.Local); err == nil {
			return t
		}
	}
	return time.Time{}
}

// jsString reads a string a script sets, as fS_name = "…".
func jsString(src, name string) string {
	i := strings.Index(src, name+" = \"")
	if i < 0 {
		return ""
	}
	rest := src[i+len(name)+4:]
	if j := strings.IndexByte(rest, '"'); j >= 0 {
		return rest[:j]
	}
	return ""
}

// jsArray reads an array a script sets, as Data_netWorthTrend = […]; it
// returns the array's JSON.
func jsArray(src, name string) string {
	i := strings.Index(src, name+" =")
	if i < 0 {
		return ""
	}
	rest := src[i+len(name)+2:]
	j := strings.IndexByte(rest, '[')
	if j < 0 {
		return ""
	}
	start := j
	depth := 0
	inString := false
	for k := j; k < len(rest); k++ {
		c := rest[k]
		switch {
		case inString:
			if c == '\\' {
				k++
			} else if c == '"' {
				inString = false
			}
		case c == '"':
			inString = true
		case c == '[':
			depth++
		case c == ']':
			depth--
			if depth == 0 {
				return rest[start : k+1]
			}
		}
	}
	return ""
}
