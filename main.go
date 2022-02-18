package main

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"path/filepath"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

type Website struct {
	ID       int    `db:"id"`
	Hostname string `db:"hostname"`
}

type File struct {
	ID        int    `db:"id"`
	WebsiteID int    `db:"website_id"`
	Path      string `db:"path"`
	Blob      []byte `db:"blob"`
}

var TABLES string = `
CREATE TABLE IF NOT EXISTS website (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    hostname VARCHAR NOT NULL
);

CREATE TABLE IF NOT EXISTS file (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    website_id INTEGER NOT NULL,
    path VARCHAR NOT NULL,
	blob BLOB NOT NULL,
    FOREIGN KEY(website_id) REFERENCES website(id)
);
`

var db *sqlx.DB

func main() {
	fmt.Println("serving your pages")
	var err error
	db, err = sqlx.Connect("sqlite", "your-pages.db")

	if err != nil {
		fmt.Printf("could not open database: %s\n", err)
	}

	// setup database
	db.MustExec(TABLES)

	mux := http.NewServeMux()
	mux.HandleFunc("/upload", UploadHandler)
	mux.HandleFunc("/", ServeHandler)

	log.Fatal(http.ListenAndServe(":4444", mux))
}

func UploadHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "only HTTP posts requests accepted", http.StatusMethodNotAllowed)
		return
	}

	r.ParseMultipartForm(32 << 20)
	sites := []string{}
	for site := range r.MultipartForm.File {
		sites = append(sites, site)
	}

	file, header, err := r.FormFile(sites[0])
	if err != nil {
		http.Error(w, fmt.Sprint("could not read file: ", err), http.StatusBadRequest)
	}

	mimeType := mime.TypeByExtension(filepath.Ext(header.Filename))
	if mimeType != "application/gzip" {
		http.Error(w, "only gzip files accepted", http.StatusBadRequest)
		return
	}

	gzipped, err := gzip.NewReader(file)
	if err != nil {
		http.Error(w, fmt.Sprint("could not create GZIP reader: ", err), http.StatusInternalServerError)
	}
	gzipped.Close()

	tarred := tar.NewReader(gzipped)

	for {
		header, err := tarred.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			http.Error(w, fmt.Sprint("could not read tar: ", err), http.StatusInternalServerError)
		}

		fmt.Printf("header: %v\n", header.Name)

		// TODO: stream file into database
		bytes, err := io.ReadAll(tarred)
		if err != nil {
			http.Error(w, fmt.Sprint("could not read tar: ", err), http.StatusInternalServerError)
		}

		file := &File{
			WebsiteID: 0,
			Path:      header.Name,
			Blob:      bytes,
		}

		db.NamedExec(`
			INSERT INTO file (website_id, path, blob)
			VALUES (:website_id, :path, :blob)
			`, file)
	}

	fmt.Fprintf(w, "Hello, there upload\n")
	// fmt.Print(mimeType, string(data[:100]))
}

func ServeHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "Hello, there serve\n")
}
