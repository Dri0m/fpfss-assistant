package main

import (
	"database/sql"
	"flag"
	"fmt"
	"github.com/joho/godotenv"
	"github.com/sirupsen/logrus"
	"io"
	"io/ioutil"
	"mime"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	dbName           = "db.db"
	filePath         = "./downloads"
	statusWaiting    = "waiting"
	statusDownloaded = "downloaded"
	statusMarked     = "marked"
	actionSearch     = "search"
	actionDownload   = "download"
	actionMarkAdded  = "mark"
	actionCheck      = "check"
)

func main() {
	a := app{}
	a.initLogger()

	a.fatalErr(godotenv.Load())
	a.getConfig(a.l)

	a.openDB()
	defer a.db.Close()

	var actionFlag string
	flag.StringVar(&actionFlag, "action", "", "command")
	var nFlag int
	flag.IntVar(&nFlag, "n", 0, fmt.Sprintf("(%s) how many submissions to search for", actionSearch))
	var ignoreFlag string
	flag.StringVar(&ignoreFlag, "ignore", "", fmt.Sprintf("(%s) comma-separated list of submission IDs to ignore instead of marking as added", actionMarkAdded))
	var rejectFlag string
	flag.StringVar(&rejectFlag, "reject", "", fmt.Sprintf("(%s) comma-separated list of submission IDs to reject instead of marking as added", actionMarkAdded))
	flag.Parse()

	switch actionFlag {
	case actionSearch:
		if nFlag < 1 {
			a.printlnf("invalid number provided (provide flag -n=N)")
			os.Exit(1)
		}
		a.search(nFlag)
	case actionDownload:
		a.download()
	case actionMarkAdded:
		ignore, err := splitParse(ignoreFlag)
		a.fatalErr(err)
		reject, err := splitParse(rejectFlag)
		a.fatalErr(err)
		a.mark(ignore, reject)
	case actionCheck:
		a.check()
	default:
		a.printlnf("unknown command '%s'", actionFlag)
		os.Exit(1)
	}

	//var count int
	//err := a.db.QueryRow("SELECT COUNT(*) FROM data").Scan(&count)
	//a.fatalErr(err)

	a.printlnf("all done'd")
}

func splitParse(s string) ([]int64, error) {
	if len(s) == 0 {
		return nil, nil
	}
	tmp := strings.Split(s, ",")
	values := make([]int64, 0, len(tmp))
	for _, raw := range tmp {
		v, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return nil, err
		}
		values = append(values, v)
	}
	return values, nil
}

type Submission struct {
	id      int64
	status  string
	fileURL string
	sha256  string
	title   string
}

type app struct {
	l      *logrus.Logger
	db     *sql.DB
	config *Config
	dbm    sync.Mutex
}

func (a *app) printlnf(format string, x ...interface{}) {
	fmt.Printf(format+"\n", x...)
	a.l.Infof(format, x...)
}

func (a *app) getResponse(url string) ([]byte, error, int) {
	var myClient = &http.Client{Timeout: 600 * time.Second}

	a.l.Debugf("getting URL '%s'...", url)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err, -1
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:85.0) Gecko/20100101 Firefox/85.0")

	cookie := &http.Cookie{
		Name:  "login",
		Value: a.config.Cookie,
	}
	req.AddCookie(cookie)

	resp, err := myClient.Do(req)
	if err != nil {
		return nil, err, -1
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("request for '%s' failed with code: %d", url, resp.StatusCode), resp.StatusCode
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err, -1
	}

	return body, nil, resp.StatusCode
}

func (a *app) getFile(url string) (io.ReadCloser, error, string) {
	var myClient = &http.Client{Timeout: 86400 * time.Second}

	a.l.Debugf("getting URL '%s'...", url)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err, ""
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:85.0) Gecko/20100101 Firefox/85.0")

	cookie := &http.Cookie{
		Name:  "login",
		Value: a.config.Cookie,
	}
	req.AddCookie(cookie)

	resp, err := myClient.Do(req)
	if err != nil {
		resp.Body.Close()
		return nil, err, ""
	}

	_, params, err := mime.ParseMediaType(resp.Header.Get("Content-Disposition"))
	filename := params["filename"]

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("request for '%s' failed with code: %d", url, resp.StatusCode), ""
	}

	return resp.Body, nil, filename
}

func (a *app) post(url string) error {
	var myClient = &http.Client{Timeout: 86400 * time.Second}

	a.l.Debugf("posting URL '%s'...", url)
	req, err := http.NewRequest("POST", url, nil)
	if err != nil {
		return err
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:85.0) Gecko/20100101 Firefox/85.0")

	cookie := &http.Cookie{
		Name:  "login",
		Value: a.config.Cookie,
	}
	req.AddCookie(cookie)

	resp, err := myClient.Do(req)
	defer resp.Body.Close()
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("request for '%s' failed with code: %d", url, resp.StatusCode)
	}

	return nil
}
