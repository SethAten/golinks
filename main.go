package main

import (
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/GeertJohan/go.rice"
	"github.com/boltdb/bolt"
	"github.com/namsral/flag"

	"go.iondynamics.net/templice"
)

var (
	db        *bolt.DB
	cfg       Config
	templates = templice.New(rice.MustFindBox("templates"))
)

// DefaultURL redirects to Google Search by default for unknown queries
const DefaultURL string = "https://www.google.com/search?q=%s&btnOk"

const openSearchTemplate string = `<?xml version="1.0" encoding="UTF-8"?>
<OpenSearchDescription xmlns="http://a9.com/-/spec/opensearch/1.1/">
  <ShortName>%s</ShortName>
  <Description>Smart bookmarks</Description>
  <Tags>search</Tags>
  <Contact>admin@localhost</Contact>
  <Url type="text/html" method="get" template="http://%s/?q={searchTerms}"/>
</OpenSearchDescription>
`

func render(w http.ResponseWriter, tmpl string, data interface{}) {
	err := templates.ExecuteTemplate(w, tmpl+".html", data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// QueryHandler ...
func QueryHandler(url string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var (
			cmd  string
			args []string
		)

		q := r.URL.Query().Get("q")
		tokens := strings.Split(q, " ")
		if len(tokens) > 0 {
			cmd, args = tokens[0], tokens[1:]
		}

		if cmd == "" {
			render(w, "index", nil)
		} else {
			if command := LookupCommand(cmd); command != nil {
				err := command.Exec(w, args)
				if err != nil {
					http.Error(
						w,
						fmt.Sprintf(
							"Error processing command %s: %s",
							command.Name(), err,
						),
						http.StatusInternalServerError,
					)
				}
			} else if bookmark, ok := LookupBookmark(cmd); ok {
				q := strings.Join(args, " ")
				bookmark.Exec(w, r, q)
			} else {
				if url != "" {
					u := url
					if q != "" {
						u = fmt.Sprintf(u, q)
					}
					http.Redirect(w, r, u, http.StatusFound)
				} else {
					http.Error(
						w,
						fmt.Sprintf("Invalid Command: %v", cmd),
						http.StatusBadRequest,
					)
				}
			}
		}
	})
}

// OpenSearchHandler ...
func OpenSearchHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/xml")
		// TODO: Abstract the Config and Handlers better
		w.Write([]byte(fmt.Sprintf(openSearchTemplate, cfg.Title, cfg.FQDN)))
	})
}

func main() {
	var (
		config string
		dbpath string
		title  string
		bind   string
		fqdn   string
		url    string
	)

	flag.StringVar(&config, "config", "", "config file")
	flag.StringVar(&dbpath, "dbpth", "search.db", "Database path")
	flag.StringVar(&title, "title", "Search", "OpenSearch Title")
	flag.StringVar(&bind, "bind", "0.0.0.0:80", "[int]:<port> to bind to")
	flag.StringVar(&fqdn, "fqdn", "localhost", "FQDN for public access")
	flag.StringVar(&url, "url", DefaultURL, "default url to redirect to")
	flag.Parse()

	// TODO: Abstract the Config and Handlers better
	cfg.Title = title
	cfg.FQDN = fqdn

	var err error
	db, err = bolt.Open(dbpath, 0600, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	templates.Load()

	err = EnsureDefaultBookmarks()
	if err != nil {
		log.Fatal(err)
	}

	http.Handle("/", QueryHandler(url))
	http.Handle("/opensearch.xml", OpenSearchHandler())
	log.Fatal(http.ListenAndServe(bind, nil))
}
